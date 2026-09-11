package api

import (
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func f64(v float64) *float64 { return &v }

func TestValidateAgentCommand(t *testing.T) {
	tests := []struct {
		name     string
		req      domain.AgentCommandRequest
		wantErrs int
	}{
		{"save takes no args", domain.AgentCommandRequest{Command: "save"}, 0},
		{"kick needs target", domain.AgentCommandRequest{Command: "kick"}, 1},
		{"kick ok", domain.AgentCommandRequest{Command: "kick", Target: "Bjorn"}, 0},
		{"broadcast needs message", domain.AgentCommandRequest{Command: "broadcast"}, 1},
		{"broadcast bad style", domain.AgentCommandRequest{Command: "broadcast", Message: "hi", Style: "middle"}, 1},
		{"say ok", domain.AgentCommandRequest{Command: "say", Message: "hi"}, 0},
		{"say name too long", domain.AgentCommandRequest{Command: "say", Message: "hi", Name: string(make([]byte, 33))}, 1},
		{"time none set", domain.AgentCommandRequest{Command: "time"}, 1},
		{"time fraction ok", domain.AgentCommandRequest{Command: "time", Fraction: f64(0.5)}, 0},
		{"time fraction out of range", domain.AgentCommandRequest{Command: "time", Fraction: f64(2)}, 1},
		{"time two set", domain.AgentCommandRequest{Command: "time", Skip: "morning", Seconds: f64(60)}, 1},
		{"time bad skip", domain.AgentCommandRequest{Command: "time", Skip: "noon"}, 1},
		{"time seconds ok", domain.AgentCommandRequest{Command: "time", Seconds: f64(3600)}, 0},
		{"setkey ok", domain.AgentCommandRequest{Command: "setkey", Key: "defeated_eikthyr"}, 0},
		{"setkey bad", domain.AgentCommandRequest{Command: "setkey", Key: "bad key!"}, 1},
		{"event ok no anchor", domain.AgentCommandRequest{Command: "event", Event: "army_eikthyr"}, 0},
		{"event needs name", domain.AgentCommandRequest{Command: "event"}, 1},
		{"event half anchor", domain.AgentCommandRequest{Command: "event", Event: "raid", X: f64(1)}, 1},
		{"event full anchor", domain.AgentCommandRequest{Command: "event", Event: "raid", X: f64(1), Z: f64(2)}, 0},
		{"eventstop no args", domain.AgentCommandRequest{Command: "eventstop"}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validateAgentCommand(tc.req)
			if len(got) != tc.wantErrs {
				t.Fatalf("got %d field errors %+v, want %d", len(got), got, tc.wantErrs)
			}
		})
	}
}
