// Package query implements the Source Engine A2S_INFO protocol over UDP,
// including the 2020+ S2C_CHALLENGE round-trip, used to poll a Valheim
// instance's query port for authoritative player counts (ARCHITECTURE.md §8).
package query

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Timeout is the per-attempt UDP round-trip deadline.
const Timeout = 2 * time.Second

// MaxRetries is how many extra attempts Info makes after the first failure.
const MaxRetries = 2

const (
	respHeaderChallenge byte = 'A'
	respHeaderInfo      byte = 'I'
)

var a2sHeader = [4]byte{0xFF, 0xFF, 0xFF, 0xFF}

// Info queries addr (host:port, typically 127.0.0.1:<queryPort>) for
// A2S_INFO and returns the parsed response. It retries up to MaxRetries
// times on any network or protocol error, each attempt bounded by Timeout
// (or ctx's deadline if tighter).
func Info(ctx context.Context, addr string) (domain.A2SInfo, error) {
	var lastErr error
	for attempt := 0; attempt <= MaxRetries; attempt++ {
		if err := ctx.Err(); err != nil {
			return domain.A2SInfo{}, err
		}
		info, err := query(ctx, addr)
		if err == nil {
			return info, nil
		}
		lastErr = err
	}
	return domain.A2SInfo{}, fmt.Errorf("a2s query %s: %w", addr, lastErr)
}

func query(ctx context.Context, addr string) (domain.A2SInfo, error) {
	qctx := ctx
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		qctx, cancel = context.WithTimeout(ctx, Timeout)
		defer cancel()
	}

	var d net.Dialer
	conn, err := d.DialContext(qctx, "udp", addr)
	if err != nil {
		return domain.A2SInfo{}, fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	deadline := time.Now().Add(Timeout)
	if dl, ok := qctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return domain.A2SInfo{}, fmt.Errorf("set deadline: %w", err)
	}

	kind, payload, err := roundTrip(conn, nil)
	if err != nil {
		return domain.A2SInfo{}, err
	}
	if kind == respHeaderChallenge {
		if len(payload) < 4 {
			return domain.A2SInfo{}, errors.New("a2s: short challenge response")
		}
		challenge := payload[:4]
		kind, payload, err = roundTrip(conn, challenge)
		if err != nil {
			return domain.A2SInfo{}, err
		}
	}
	if kind != respHeaderInfo {
		return domain.A2SInfo{}, fmt.Errorf("a2s: unexpected response type 0x%02x", kind)
	}
	return parseInfo(payload)
}

// roundTrip sends one A2S_INFO request (optionally with a challenge appended)
// and returns the response's type byte and payload (header stripped).
func roundTrip(conn net.Conn, challenge []byte) (byte, []byte, error) {
	if _, err := conn.Write(buildRequest(challenge)); err != nil {
		return 0, nil, fmt.Errorf("send: %w", err)
	}
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return 0, nil, fmt.Errorf("recv: %w", err)
	}
	return splitHeader(buf[:n])
}

func buildRequest(challenge []byte) []byte {
	req := make([]byte, 0, 4+1+len("Source Engine Query\x00")+len(challenge))
	req = append(req, a2sHeader[:]...)
	req = append(req, 'T')
	req = append(req, []byte("Source Engine Query\x00")...)
	req = append(req, challenge...)
	return req
}

func splitHeader(data []byte) (byte, []byte, error) {
	if len(data) < 5 || data[0] != 0xFF || data[1] != 0xFF || data[2] != 0xFF || data[3] != 0xFF {
		return 0, nil, errors.New("a2s: invalid response header")
	}
	return data[4], data[5:], nil
}

// byteReader is a minimal cursor over an A2S payload.
type byteReader struct {
	b []byte
	i int
}

func (r *byteReader) readByte() (byte, error) {
	if r.i >= len(r.b) {
		return 0, errors.New("a2s: unexpected end of payload")
	}
	v := r.b[r.i]
	r.i++
	return v, nil
}

// skip advances past n bytes the caller does not need to interpret (e.g. the
// app id short, which A2S defines as signed but this client never uses).
func (r *byteReader) skip(n int) error {
	if r.i+n > len(r.b) {
		return errors.New("a2s: unexpected end of payload")
	}
	r.i += n
	return nil
}

func (r *byteReader) readCString() (string, error) {
	start := r.i
	for r.i < len(r.b) {
		if r.b[r.i] == 0 {
			s := string(r.b[start:r.i])
			r.i++
			return s, nil
		}
		r.i++
	}
	return "", errors.New("a2s: unterminated string")
}

// parseInfo decodes an A2S_INFO 'I' response body (Source engine format:
// protocol, name, map, folder, game, id, players, max_players, bots,
// server_type, environment, visibility, vac, version[, EDF...]). EDF and
// anything after the version string is ignored.
func parseInfo(b []byte) (domain.A2SInfo, error) {
	r := &byteReader{b: b}

	if _, err := r.readByte(); err != nil { // protocol
		return domain.A2SInfo{}, err
	}
	name, err := r.readCString()
	if err != nil {
		return domain.A2SInfo{}, err
	}
	mapName, err := r.readCString()
	if err != nil {
		return domain.A2SInfo{}, err
	}
	if _, err := r.readCString(); err != nil { // folder
		return domain.A2SInfo{}, err
	}
	if _, err := r.readCString(); err != nil { // game
		return domain.A2SInfo{}, err
	}
	if err := r.skip(2); err != nil { // app id (short, unused)
		return domain.A2SInfo{}, err
	}
	players, err := r.readByte()
	if err != nil {
		return domain.A2SInfo{}, err
	}
	maxPlayers, err := r.readByte()
	if err != nil {
		return domain.A2SInfo{}, err
	}
	if _, err := r.readByte(); err != nil { // bots
		return domain.A2SInfo{}, err
	}
	if _, err := r.readByte(); err != nil { // server type
		return domain.A2SInfo{}, err
	}
	if _, err := r.readByte(); err != nil { // environment
		return domain.A2SInfo{}, err
	}
	visibility, err := r.readByte()
	if err != nil {
		return domain.A2SInfo{}, err
	}
	if _, err := r.readByte(); err != nil { // vac
		return domain.A2SInfo{}, err
	}
	version, err := r.readCString()
	if err != nil {
		return domain.A2SInfo{}, err
	}
	// EDF and anything after it is intentionally ignored.

	return domain.A2SInfo{
		ServerName:        name,
		Map:               mapName,
		Players:           int(players),
		MaxPlayers:        int(maxPlayers),
		Version:           version,
		PasswordProtected: visibility == 1,
		QueriedAt:         time.Now(),
	}, nil
}
