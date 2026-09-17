package domain

import "testing"

func f64(v float64) *float64 { return &v }

func TestValidateAgentCommand(t *testing.T) {
	tests := []struct {
		name     string
		req      AgentCommandRequest
		wantErrs int
	}{
		{"save takes no args", AgentCommandRequest{Command: "save"}, 0},
		{"kick needs target", AgentCommandRequest{Command: "kick"}, 1},
		{"kick ok", AgentCommandRequest{Command: "kick", Target: "Bjorn"}, 0},
		{"broadcast needs message", AgentCommandRequest{Command: "broadcast"}, 1},
		{"broadcast bad style", AgentCommandRequest{Command: "broadcast", Message: "hi", Style: "middle"}, 1},
		{"say ok", AgentCommandRequest{Command: "say", Message: "hi"}, 0},
		{"say name too long", AgentCommandRequest{Command: "say", Message: "hi", Name: string(make([]byte, 33))}, 1},
		{"time none set", AgentCommandRequest{Command: "time"}, 1},
		{"time fraction ok", AgentCommandRequest{Command: "time", Fraction: f64(0.5)}, 0},
		{"time fraction out of range", AgentCommandRequest{Command: "time", Fraction: f64(2)}, 1},
		{"time two set", AgentCommandRequest{Command: "time", Skip: "morning", Seconds: f64(60)}, 1},
		{"time bad skip", AgentCommandRequest{Command: "time", Skip: "noon"}, 1},
		{"time seconds ok", AgentCommandRequest{Command: "time", Seconds: f64(3600)}, 0},
		{"setkey ok", AgentCommandRequest{Command: "setkey", Key: "defeated_eikthyr"}, 0},
		{"setkey bad", AgentCommandRequest{Command: "setkey", Key: "bad key!"}, 1},
		{"event ok no anchor", AgentCommandRequest{Command: "event", Event: "army_eikthyr"}, 0},
		{"event needs name", AgentCommandRequest{Command: "event"}, 1},
		{"event half anchor", AgentCommandRequest{Command: "event", Event: "raid", X: f64(1)}, 1},
		{"event full anchor", AgentCommandRequest{Command: "event", Event: "raid", X: f64(1), Z: f64(2)}, 0},
		{"eventstop no args", AgentCommandRequest{Command: "eventstop"}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateAgentCommand(tc.req)
			if len(got) != tc.wantErrs {
				t.Fatalf("got %d field errors %+v, want %d", len(got), got, tc.wantErrs)
			}
		})
	}
}
