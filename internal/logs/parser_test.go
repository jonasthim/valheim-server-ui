package logs

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParse_EachEventType(t *testing.T) {
	cases := []struct {
		name string
		line string
		want Event
	}{
		{
			"ready",
			`09/09/2026 10:00:03: Game server connected`,
			Ready{base{mustTime(t, "09/09/2026 10:00:03")}},
		},
		{
			"join code",
			`09/09/2026 10:00:04: Session "Our Server" with join code 123456 and IP 203.0.113.10:2456 is active with 2 player(s)`,
			JoinCode{base{mustTime(t, "09/09/2026 10:00:04")}, "123456", 2},
		},
		{
			"connected via steamid",
			`09/09/2026 10:00:10: Got connection SteamID 76561198000000001`,
			Connected{base{mustTime(t, "09/09/2026 10:00:10")}, "76561198000000001"},
		},
		{
			"connected via handshake",
			`09/09/2026 10:00:10: Got handshake from client 76561198000000001`,
			Connected{base{mustTime(t, "09/09/2026 10:00:10")}, "76561198000000001"},
		},
		{
			"spawned with spaced name",
			`09/09/2026 10:00:12: Got character ZDOID from Bjorn The Bold : 12345:1`,
			Spawned{base{mustTime(t, "09/09/2026 10:00:12")}, "Bjorn The Bold"},
		},
		{
			"despawned on zdoid zero",
			`09/09/2026 10:05:00: Got character ZDOID from Bjorn : 0:0`,
			Despawned{base{mustTime(t, "09/09/2026 10:05:00")}, "Bjorn"},
		},
		{
			"disconnected via closing socket",
			`09/09/2026 10:05:02: Closing socket 76561198000000001`,
			Disconnected{base{mustTime(t, "09/09/2026 10:05:02")}, "76561198000000001"},
		},
		{
			"disconnected via wrong password",
			`09/09/2026 10:05:03: Peer 76561198000000002 has wrong password`,
			Disconnected{base{mustTime(t, "09/09/2026 10:05:03")}, "76561198000000002"},
		},
		{
			"saved",
			`09/09/2026 10:00:40: World saved ( 42.1ms )`,
			Saved{base{mustTime(t, "09/09/2026 10:00:40")}, 42_100_000},
		},
		{
			"shutdown",
			`09/09/2026 10:10:00: OnApplicationQuit`,
			Shutdown{base{mustTime(t, "09/09/2026 10:10:00")}},
		},
		{
			"no timestamp prefix still parses",
			`Game server connected`,
			Ready{base{time.Time{}}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Parse(tc.line)
			if !ok {
				t.Fatalf("Parse(%q) = not ok, want %#v", tc.line, tc.want)
			}
			if got != tc.want {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.line, got, tc.want)
			}
		})
	}
}

func TestParse_UnknownLinesNeverError(t *testing.T) {
	lines := []string{
		"",
		"Starting server PRESS CTRL-C to exit",
		"some line with no timestamp at all",
		"09/09/2026 10:00:01: Valheim version: 0.220.5 (fake) DOORSTOP_ENABLED=unset SteamAppId=892970",
		"09/09/2026 10:00:02: Zonesystem Start 0",
		": weird empty timestamp line",
		"09/09/2026 99/99/9999 not a real message",
		"\x00\x01 binary garbage \xff",
	}
	for _, l := range lines {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Parse(%q) panicked: %v", l, r)
				}
			}()
			if ev, ok := Parse(l); ok {
				t.Fatalf("Parse(%q) = %#v, ok=true, want no event", l, ev)
			}
		}()
	}
}

func TestParse_Fixtures(t *testing.T) {
	// Every non-blank line across the fixtures must parse without panicking;
	// fixtures that are meant to contain known events must produce at least
	// one of each kind exercised by their name.
	files := map[string]func(t *testing.T, events []Event){
		"startup.log": func(t *testing.T, events []Event) {
			wantKinds(t, events, "logs.Ready", "logs.JoinCode")
		},
		"session.log": func(t *testing.T, events []Event) {
			wantKinds(t, events, "logs.Connected", "logs.Spawned", "logs.Despawned",
				"logs.JoinCode", "logs.Saved", "logs.Disconnected")
		},
		"shutdown.log": func(t *testing.T, events []Event) {
			wantKinds(t, events, "logs.Shutdown", "logs.Saved")
		},
		"fakeserver.log": func(t *testing.T, events []Event) {
			wantKinds(t, events, "logs.Ready", "logs.JoinCode", "logs.Connected",
				"logs.Spawned", "logs.Disconnected", "logs.Saved", "logs.Shutdown")
		},
	}

	for name, check := range files {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join("testdata", name))
			if err != nil {
				t.Fatalf("open fixture: %v", err)
			}
			defer f.Close()
			var events []Event
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				line := sc.Text()
				if line == "" {
					continue
				}
				if ev, ok := Parse(line); ok {
					events = append(events, ev)
				}
			}
			if err := sc.Err(); err != nil {
				t.Fatalf("scan fixture: %v", err)
			}
			check(t, events)
		})
	}

	t.Run("unknown.log never errors", func(t *testing.T) {
		f, err := os.Open(filepath.Join("testdata", "unknown.log"))
		if err != nil {
			t.Fatalf("open fixture: %v", err)
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			_, _ = Parse(sc.Text()) // must not panic
		}
		if err := sc.Err(); err != nil {
			t.Fatalf("scan fixture: %v", err)
		}
	})
}

func wantKinds(t *testing.T, events []Event, kinds ...string) {
	t.Helper()
	seen := map[string]bool{}
	for _, ev := range events {
		seen[eventKind(ev)] = true
	}
	for _, k := range kinds {
		if !seen[k] {
			t.Errorf("fixture did not produce a %s event; got kinds %v", k, seen)
		}
	}
}

func eventKind(ev Event) string {
	switch ev.(type) {
	case Ready:
		return "logs.Ready"
	case JoinCode:
		return "logs.JoinCode"
	case Connected:
		return "logs.Connected"
	case Spawned:
		return "logs.Spawned"
	case Despawned:
		return "logs.Despawned"
	case Disconnected:
		return "logs.Disconnected"
	case Saved:
		return "logs.Saved"
	case Shutdown:
		return "logs.Shutdown"
	default:
		return "unknown"
	}
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(tsLayout, s)
	if err != nil {
		t.Fatalf("parse time %q: %v", s, err)
	}
	return tm
}
