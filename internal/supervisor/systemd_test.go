package supervisor

import (
	"testing"
	"time"
)

func TestParseSystemctlShow(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want Status
	}{
		{
			name: "inactive",
			out: "ActiveState=inactive\nSubState=dead\nMainPID=0\n" +
				"ExecMainStartTimestamp=\nResult=success\nUnitFileState=disabled\n",
			want: Status{State: StateStopped, PID: 0, Autostart: false},
		},
		{
			name: "activating",
			out: "ActiveState=activating\nSubState=start\nMainPID=4821\n" +
				"ExecMainStartTimestamp=Tue 2024-01-16 20:31:12 UTC\nResult=success\nUnitFileState=enabled\n",
			want: Status{State: StateStarting, PID: 4821, Autostart: true,
				Since: mustParseTS(t, "Tue 2024-01-16 20:31:12 UTC")},
		},
		{
			name: "active running",
			out: "ActiveState=active\nSubState=running\nMainPID=4821\n" +
				"ExecMainStartTimestamp=Tue 2024-01-16 20:31:14 UTC\nResult=success\nUnitFileState=enabled\n",
			want: Status{State: StateRunning, PID: 4821, Autostart: true,
				Since: mustParseTS(t, "Tue 2024-01-16 20:31:14 UTC")},
		},
		{
			name: "deactivating",
			out: "ActiveState=deactivating\nSubState=stop-sigterm\nMainPID=4821\n" +
				"ExecMainStartTimestamp=Tue 2024-01-16 20:31:14 UTC\nResult=success\nUnitFileState=enabled\n",
			want: Status{State: StateStopping, PID: 4821, Autostart: true,
				Since: mustParseTS(t, "Tue 2024-01-16 20:31:14 UTC")},
		},
		{
			name: "failed with exit-code result",
			out: "ActiveState=failed\nSubState=failed\nMainPID=0\n" +
				"ExecMainStartTimestamp=Tue 2024-01-16 20:31:14 UTC\nResult=exit-code\nUnitFileState=enabled\n",
			want: Status{State: StateFailed, PID: 0, Autostart: true, Detail: "exit-code",
				Since: mustParseTS(t, "Tue 2024-01-16 20:31:14 UTC")},
		},
		{
			name: "unparseable timestamp is zero time",
			out: "ActiveState=active\nSubState=running\nMainPID=99\n" +
				"ExecMainStartTimestamp=garbage\nResult=success\nUnitFileState=static\n",
			want: Status{State: StateRunning, PID: 99, Autostart: true},
		},
		{
			name: "disabled unit file state",
			out: "ActiveState=active\nSubState=running\nMainPID=99\n" +
				"ExecMainStartTimestamp=\nResult=success\nUnitFileState=disabled\n",
			want: Status{State: StateRunning, PID: 99, Autostart: false},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSystemctlShow(tc.out)
			if got.State != tc.want.State {
				t.Errorf("State = %v, want %v", got.State, tc.want.State)
			}
			if got.PID != tc.want.PID {
				t.Errorf("PID = %v, want %v", got.PID, tc.want.PID)
			}
			if got.Autostart != tc.want.Autostart {
				t.Errorf("Autostart = %v, want %v", got.Autostart, tc.want.Autostart)
			}
			if got.Detail != tc.want.Detail {
				t.Errorf("Detail = %q, want %q", got.Detail, tc.want.Detail)
			}
			if !got.Since.Equal(tc.want.Since) {
				t.Errorf("Since = %v, want %v", got.Since, tc.want.Since)
			}
		})
	}
}

func mustParseTS(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(systemTimestampLayout, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}
