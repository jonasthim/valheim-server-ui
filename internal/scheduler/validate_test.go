package scheduler

import (
	"strings"
	"testing"

	"github.com/robfig/cron/v3"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestValidateInput(t *testing.T) {
	parser := cron.NewParser(cronParseOptions)

	cases := []struct {
		name    string
		in      domain.ScheduleInput
		wantErr []string // field names expected to have an error; nil means valid
	}{
		{
			name: "valid standard 5-field",
			in:   domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "0 3 * * *", Enabled: true},
		},
		{
			name: "valid every minute",
			in:   domain.ScheduleInput{Kind: domain.ScheduleRestart, Cron: "* * * * *", Enabled: true},
		},
		{
			name: "valid day-of-week list",
			in:   domain.ScheduleInput{Kind: domain.ScheduleUpdate, Cron: "0 3 * * mon,wed,fri", Enabled: true},
		},
		{
			name: "valid descriptor",
			in:   domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "@daily", Enabled: true},
		},
		{
			name: "valid hourly descriptor",
			in:   domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "@hourly", Enabled: true},
		},
		{
			name: "valid with max-length note",
			in:   domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "@daily", Note: strings.Repeat("n", 100)},
		},
		{
			name:    "unknown kind",
			in:      domain.ScheduleInput{Kind: "frobnicate", Cron: "@daily"},
			wantErr: []string{"kind"},
		},
		{
			name:    "empty kind",
			in:      domain.ScheduleInput{Kind: "", Cron: "@daily"},
			wantErr: []string{"kind"},
		},
		{
			name:    "empty cron",
			in:      domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: ""},
			wantErr: []string{"cron"},
		},
		{
			name:    "garbage cron",
			in:      domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "not a cron expression"},
			wantErr: []string{"cron"},
		},
		{
			name:    "six fields (seconds not accepted)",
			in:      domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "* * * * * *"},
			wantErr: []string{"cron"},
		},
		{
			name:    "out of range field",
			in:      domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "0 25 * * *"},
			wantErr: []string{"cron"},
		},
		{
			name:    "note too long",
			in:      domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "@daily", Note: strings.Repeat("n", 101)},
			wantErr: []string{"note"},
		},
		{
			name:    "kind and cron both invalid",
			in:      domain.ScheduleInput{Kind: "bogus", Cron: ""},
			wantErr: []string{"kind", "cron"},
		},
		{
			name: "valid announce",
			in:   domain.ScheduleInput{Kind: domain.ScheduleAnnounce, Cron: "@daily", Enabled: true, Message: "Server restarting soon"},
		},
		{
			name: "valid announce with max-length message",
			in:   domain.ScheduleInput{Kind: domain.ScheduleAnnounce, Cron: "@daily", Message: strings.Repeat("m", 500)},
		},
		{
			name:    "announce missing message",
			in:      domain.ScheduleInput{Kind: domain.ScheduleAnnounce, Cron: "@daily"},
			wantErr: []string{"message"},
		},
		{
			name:    "announce blank message",
			in:      domain.ScheduleInput{Kind: domain.ScheduleAnnounce, Cron: "@daily", Message: "   "},
			wantErr: []string{"message"},
		},
		{
			name:    "announce message too long",
			in:      domain.ScheduleInput{Kind: domain.ScheduleAnnounce, Cron: "@daily", Message: strings.Repeat("m", 501)},
			wantErr: []string{"message"},
		},
		{
			name: "valid command",
			in:   domain.ScheduleInput{Kind: domain.ScheduleCommand, Cron: "@daily", Command: &domain.AgentCommandRequest{Command: "save"}},
		},
		{
			name:    "command missing",
			in:      domain.ScheduleInput{Kind: domain.ScheduleCommand, Cron: "@daily"},
			wantErr: []string{"command"},
		},
		{
			name:    "command with invalid args surfaces prefixed field",
			in:      domain.ScheduleInput{Kind: domain.ScheduleCommand, Cron: "@daily", Command: &domain.AgentCommandRequest{Command: "kick"}},
			wantErr: []string{"command.target"},
		},
		{
			name:    "command with an unknown name is rejected at save time",
			in:      domain.ScheduleInput{Kind: domain.ScheduleCommand, Cron: "@daily", Command: &domain.AgentCommandRequest{Command: "reboot"}},
			wantErr: []string{"command.command"},
		},
		{
			name: "valid save",
			in:   domain.ScheduleInput{Kind: domain.ScheduleSave, Cron: "@daily"},
		},
		{
			name: "valid restart with lead_seconds",
			in:   domain.ScheduleInput{Kind: domain.ScheduleRestart, Cron: "@daily", LeadSeconds: 30},
		},
		{
			name: "valid restart with zero lead_seconds (use default)",
			in:   domain.ScheduleInput{Kind: domain.ScheduleRestart, Cron: "@daily", LeadSeconds: 0},
		},
		{
			name: "valid restart with max lead_seconds",
			in:   domain.ScheduleInput{Kind: domain.ScheduleRestart, Cron: "@daily", LeadSeconds: 3600},
		},
		{
			name:    "restart lead_seconds too high",
			in:      domain.ScheduleInput{Kind: domain.ScheduleRestart, Cron: "@daily", LeadSeconds: 3601},
			wantErr: []string{"lead_seconds"},
		},
		{
			name:    "restart lead_seconds negative",
			in:      domain.ScheduleInput{Kind: domain.ScheduleRestart, Cron: "@daily", LeadSeconds: -1},
			wantErr: []string{"lead_seconds"},
		},
		{
			name:    "backup must not carry a message",
			in:      domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "@daily", Message: "nope"},
			wantErr: []string{"message"},
		},
		{
			name:    "restart must not carry a command",
			in:      domain.ScheduleInput{Kind: domain.ScheduleRestart, Cron: "@daily", Command: &domain.AgentCommandRequest{Command: "save"}},
			wantErr: []string{"command"},
		},
		{
			name:    "update must not carry a message or command",
			in:      domain.ScheduleInput{Kind: domain.ScheduleUpdate, Cron: "@daily", Message: "nope", Command: &domain.AgentCommandRequest{Command: "save"}},
			wantErr: []string{"message", "command"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := validateInput(tc.in, parser)
			if len(tc.wantErr) == 0 {
				if len(fs) != 0 {
					t.Fatalf("expected no errors, got %+v", fs)
				}
				return
			}
			got := map[string]bool{}
			for _, f := range fs {
				got[f.Field] = true
			}
			for _, want := range tc.wantErr {
				if !got[want] {
					t.Errorf("expected an error on field %q, got %+v", want, fs)
				}
			}
			if len(got) != len(tc.wantErr) {
				t.Errorf("unexpected extra error fields: %+v", fs)
			}
		})
	}
}
