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
