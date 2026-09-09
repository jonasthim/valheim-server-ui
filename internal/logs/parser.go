package logs

import (
	"regexp"
	"strconv"
	"time"
)

// Event is a typed message extracted from a console.log line. Concrete types
// below are the only implementations.
type Event interface {
	isLogEvent()
	// When returns the line's timestamp, or the zero Time when the line had
	// no "MM/DD/YYYY HH:MM:SS: " prefix.
	When() time.Time
}

type base struct{ At time.Time }

func (b base) When() time.Time { return b.At }

// Ready is emitted for "Game server connected": the world has loaded and the
// server is accepting players.
type Ready struct{ base }

// JoinCode is emitted for the "Session ... join code ..." line (crossplay).
type JoinCode struct {
	base
	Code    string
	Players int
}

// Connected is emitted when a client opens a connection, from either the
// SteamID or handshake line. ID is the platform id as it appears in the log.
type Connected struct {
	base
	ID string
}

// Spawned is emitted when a player's character appears in the world.
type Spawned struct {
	base
	Name string
}

// Despawned is emitted for a ZDOID of "0:0", meaning the character despawned
// (logout or death transition), not that the connection closed.
type Despawned struct {
	base
	Name string
}

// Disconnected is emitted when a connection closes, from either the socket
// or wrong-password line.
type Disconnected struct {
	base
	ID string
}

// Saved is emitted when the world finishes saving.
type Saved struct {
	base
	Duration time.Duration
}

// Shutdown is emitted for "OnApplicationQuit".
type Shutdown struct{ base }

func (Ready) isLogEvent()        {}
func (JoinCode) isLogEvent()     {}
func (Connected) isLogEvent()    {}
func (Spawned) isLogEvent()      {}
func (Despawned) isLogEvent()    {}
func (Disconnected) isLogEvent() {}
func (Saved) isLogEvent()        {}
func (Shutdown) isLogEvent()     {}

// tsPrefix matches the optional "MM/DD/YYYY HH:MM:SS: " prefix Valheim
// prints on (most) lines.
var tsPrefix = regexp.MustCompile(`^(\d{2}/\d{2}/\d{4} \d{2}:\d{2}:\d{2}): (.*)$`)

const tsLayout = "01/02/2006 15:04:05"

type rule struct {
	re    *regexp.Regexp
	build func(m []string, at time.Time) Event
}

// rules is intentionally table-driven: Valheim's own log lines drift between
// patches, so parsing must degrade to "no event" rather than fail. Order
// matters only in that the first match wins; none of these patterns overlap.
var rules = []rule{
	{
		re: regexp.MustCompile(`^Game server connected$`),
		build: func(_ []string, at time.Time) Event {
			return Ready{base{at}}
		},
	},
	{
		// Session "My Server" with join code 123456 and IP 203.0.113.10:2456 is active with 2 player(s)
		re: regexp.MustCompile(`^Session ".*" with join code (\d+) and IP [^:]+:\d+ is active with (\d+) player\(s\)$`),
		build: func(m []string, at time.Time) Event {
			n, _ := strconv.Atoi(m[2])
			return JoinCode{base{at}, m[1], n}
		},
	},
	{
		re: regexp.MustCompile(`^Got connection SteamID (\d+)$`),
		build: func(m []string, at time.Time) Event {
			return Connected{base{at}, m[1]}
		},
	},
	{
		re: regexp.MustCompile(`^Got handshake from client (\d+)$`),
		build: func(m []string, at time.Time) Event {
			return Connected{base{at}, m[1]}
		},
	},
	{
		// Got character ZDOID from Bjorn : 12345:1   (name may contain spaces)
		re: regexp.MustCompile(`^Got character ZDOID from (.+) : (-?\d+):(-?\d+)$`),
		build: func(m []string, at time.Time) Event {
			if m[2] == "0" && m[3] == "0" {
				return Despawned{base{at}, m[1]}
			}
			return Spawned{base{at}, m[1]}
		},
	},
	{
		re: regexp.MustCompile(`^Closing socket (\d+)$`),
		build: func(m []string, at time.Time) Event {
			return Disconnected{base{at}, m[1]}
		},
	},
	{
		re: regexp.MustCompile(`^Peer (\d+) has wrong password$`),
		build: func(m []string, at time.Time) Event {
			return Disconnected{base{at}, m[1]}
		},
	},
	{
		re: regexp.MustCompile(`^World saved \( ([\d.]+)ms \)$`),
		build: func(m []string, at time.Time) Event {
			ms, _ := strconv.ParseFloat(m[1], 64)
			return Saved{base{at}, time.Duration(ms * float64(time.Millisecond))}
		},
	},
	{
		re: regexp.MustCompile(`^OnApplicationQuit$`),
		build: func(_ []string, at time.Time) Event {
			return Shutdown{base{at}}
		},
	},
}

// Parse extracts a typed Event from one console.log line. It returns
// (nil, false) for lines it does not recognise; it never errors or panics.
func Parse(line string) (Event, bool) {
	msg := line
	var at time.Time
	if m := tsPrefix.FindStringSubmatch(line); m != nil {
		if t, err := time.Parse(tsLayout, m[1]); err == nil {
			at = t
		}
		msg = m[2]
	}
	for _, r := range rules {
		if m := r.re.FindStringSubmatch(msg); m != nil {
			return r.build(m, at), true
		}
	}
	return nil, false
}
