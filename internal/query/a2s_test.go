package query

import (
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// fakeServer answers A2S_INFO with a challenge on the first request and a
// synthesized 'I' response once the challenge is echoed back, mimicking the
// 2020+ Source engine behaviour.
type fakeServer struct {
	conn      *net.UDPConn
	challenge [4]byte
	infoBody  []byte
	failFirst int // number of requests to silently ignore before answering
}

func newFakeServer(t *testing.T, infoBody []byte) *fakeServer {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	fs := &fakeServer{conn: conn, infoBody: infoBody}
	binary.LittleEndian.PutUint32(fs.challenge[:], 0xC0FFEE01)
	return fs
}

func (fs *fakeServer) addr() string { return fs.conn.LocalAddr().String() }

func (fs *fakeServer) close() { _ = fs.conn.Close() }

// serveOnce handles requests until ctx is done, replying with a challenge to
// a bare request and the info body once the matching challenge is echoed.
func (fs *fakeServer) serve(t *testing.T, ctx context.Context) {
	t.Helper()
	buf := make([]byte, 4096)
	for {
		if ctx.Err() != nil {
			return
		}
		_ = fs.conn.SetReadDeadline(time.Now().Add(200 * time.Millisecond))
		n, raddr, err := fs.conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		req := buf[:n]
		if fs.failFirst > 0 {
			fs.failFirst--
			continue
		}
		const prefixLen = 4 + 1 + len("Source Engine Query\x00")
		if n < prefixLen {
			continue
		}
		hasChallenge := n > prefixLen
		if hasChallenge && string(req[prefixLen:]) == string(fs.challenge[:]) {
			resp := append([]byte{0xFF, 0xFF, 0xFF, 0xFF, 'I'}, fs.infoBody...)
			_, _ = fs.conn.WriteToUDP(resp, raddr)
			continue
		}
		resp := append([]byte{0xFF, 0xFF, 0xFF, 0xFF, 'A'}, fs.challenge[:]...)
		_, _ = fs.conn.WriteToUDP(resp, raddr)
	}
}

func buildInfoBody(t *testing.T, name, mapName, folder, game string, appID int16, players, maxPlayers, bots byte, serverType, env byte, visibility, vac byte, version string) []byte {
	t.Helper()
	var b []byte
	b = append(b, 17) // protocol
	b = append(b, []byte(name)...)
	b = append(b, 0)
	b = append(b, []byte(mapName)...)
	b = append(b, 0)
	b = append(b, []byte(folder)...)
	b = append(b, 0)
	b = append(b, []byte(game)...)
	b = append(b, 0)
	idBuf := make([]byte, 2)
	binary.LittleEndian.PutUint16(idBuf, uint16(appID))
	b = append(b, idBuf...)
	b = append(b, players, maxPlayers, bots, serverType, env, visibility, vac)
	b = append(b, []byte(version)...)
	b = append(b, 0)
	return b
}

func TestInfo_ChallengeThenInfo(t *testing.T) {
	body := buildInfoBody(t, "Our Server", "world", "valheim", "Valheim", 0, 2, 10, 0, 'd', 'l', 0, 0, "0.220.5")
	fs := newFakeServer(t, body)
	defer fs.close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go fs.serve(t, ctx)

	info, err := Info(context.Background(), fs.addr())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.ServerName != "Our Server" {
		t.Errorf("ServerName = %q, want %q", info.ServerName, "Our Server")
	}
	if info.Map != "world" {
		t.Errorf("Map = %q, want %q", info.Map, "world")
	}
	if info.Players != 2 {
		t.Errorf("Players = %d, want 2", info.Players)
	}
	if info.MaxPlayers != 10 {
		t.Errorf("MaxPlayers = %d, want 10", info.MaxPlayers)
	}
	if info.Version != "0.220.5" {
		t.Errorf("Version = %q, want %q", info.Version, "0.220.5")
	}
	if info.PasswordProtected {
		t.Errorf("PasswordProtected = true, want false")
	}
	if info.QueriedAt.IsZero() {
		t.Errorf("QueriedAt is zero")
	}
}

func TestInfo_PasswordProtected(t *testing.T) {
	body := buildInfoBody(t, "Locked Server", "world", "valheim", "Valheim", 0, 0, 10, 0, 'd', 'l', 1, 0, "0.220.5")
	fs := newFakeServer(t, body)
	defer fs.close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go fs.serve(t, ctx)

	info, err := Info(context.Background(), fs.addr())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if !info.PasswordProtected {
		t.Errorf("PasswordProtected = false, want true")
	}
}

func TestInfo_RetriesOnTransientFailure(t *testing.T) {
	body := buildInfoBody(t, "Retry Server", "world", "valheim", "Valheim", 0, 1, 4, 0, 'd', 'l', 0, 0, "0.220.5")
	fs := newFakeServer(t, body)
	fs.failFirst = 2 // drop the first two requests entirely (one retry's worth)
	defer fs.close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go fs.serve(t, ctx)

	info, err := Info(context.Background(), fs.addr())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.ServerName != "Retry Server" {
		t.Errorf("ServerName = %q, want %q", info.ServerName, "Retry Server")
	}
}

func TestInfo_NoResponder(t *testing.T) {
	// Bind a socket, then close it immediately so nothing is listening there.
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, err = Info(ctx, addr)
	if err == nil {
		t.Fatalf("Info: want error querying an unbound port, got nil")
	}
	if elapsed := time.Since(start); elapsed > 7*time.Second {
		t.Errorf("Info took %s, want it bounded by ~%d attempts * %s", elapsed, MaxRetries+1, Timeout)
	}
}
