package notify

import (
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestAlertsFor(t *testing.T) {
	now := time.Now()

	cases := []struct {
		name       string
		ev         domain.Event
		prevOnline []string // seeds the players snapshot before ev, for player cases
		wantKinds  []string
	}{
		{
			name: "instance crashed",
			ev: domain.Event{Name: domain.EventInstanceCrashed, InstanceID: "main", TS: now,
				Data: domain.InstanceEvent{InstanceID: "main", Kind: "crash", Detail: "exit status 1"}},
			wantKinds: []string{domain.AlertCrashed},
		},
		{
			name: "instance status failed",
			ev: domain.Event{Name: domain.EventInstanceStatus, InstanceID: "main", TS: now,
				Data: domain.InstanceStatus{InstanceID: "main", State: domain.StateFailed, Detail: "exit code 1"}},
			wantKinds: []string{domain.AlertDown},
		},
		{
			name: "instance status running produces nothing",
			ev: domain.Event{Name: domain.EventInstanceStatus, InstanceID: "main", TS: now,
				Data: domain.InstanceStatus{InstanceID: "main", State: domain.StateRunning}},
			wantKinds: nil,
		},
		{
			name: "job failed (backup) maps to job_failed",
			ev: domain.Event{Name: domain.EventJobUpdated, InstanceID: "main", TS: now,
				Data: domain.Job{Type: domain.JobBackup, InstanceID: "main", Status: domain.JobFailed, Error: "disk full"}},
			wantKinds: []string{domain.AlertJobFailed},
		},
		{
			name: "job failed (mod_install) is not an alertable job type",
			ev: domain.Event{Name: domain.EventJobUpdated, InstanceID: "main", TS: now,
				Data: domain.Job{Type: domain.JobModInstall, InstanceID: "main", Status: domain.JobFailed, Error: "boom"}},
			wantKinds: nil,
		},
		{
			name: "job failed (backup_upload) maps to job_failed",
			ev: domain.Event{Name: domain.EventJobUpdated, InstanceID: "main", TS: now,
				Data: domain.Job{Type: domain.JobBackupUpload, InstanceID: "main", Status: domain.JobFailed, Error: "rclone copyto: boom"}},
			wantKinds: []string{domain.AlertJobFailed},
		},
		{
			name: "a job.updated succeeded produces nothing",
			ev: domain.Event{Name: domain.EventJobUpdated, InstanceID: "main", TS: now,
				Data: domain.Job{Type: domain.JobBackup, InstanceID: "main", Status: domain.JobSucceeded}},
			wantKinds: nil,
		},
		{
			name: "game update available",
			ev: domain.Event{Name: domain.EventUpdateAvailable, InstanceID: "main", TS: now,
				Data: domain.UpdateInfo{InstanceID: "main", UpdateAvailable: true, LatestBuildID: "2", InstalledBuildID: "1"}},
			wantKinds: []string{domain.AlertGameUpdate},
		},
		{
			name: "app update available",
			ev: domain.Event{Name: domain.EventAppUpdateAvailable, TS: now,
				Data: domain.AppUpdateInfo{CurrentVersion: "v1.0.0", LatestVersion: "v1.1.0", UpdateAvailable: true}},
			wantKinds: []string{domain.AlertAppUpdate},
		},
		{
			name: "player joined",
			ev: domain.Event{Name: domain.EventInstancePlayers, InstanceID: "main", TS: now,
				Data: map[string]any{"instance_id": "main", "online": []domain.OnlinePlayer{{Name: "Alice"}}}},
			wantKinds: []string{domain.AlertPlayerJoin},
		},
		{
			name: "player left",
			ev: domain.Event{Name: domain.EventInstancePlayers, InstanceID: "main", TS: now,
				Data: map[string]any{"instance_id": "main", "online": []domain.OnlinePlayer{}}},
			prevOnline: []string{"Alice"},
			wantKinds:  []string{domain.AlertPlayerLeave},
		},
		{
			name: "a players event with no change produces nothing",
			ev: domain.Event{Name: domain.EventInstancePlayers, InstanceID: "main", TS: now,
				Data: map[string]any{"instance_id": "main", "online": []domain.OnlinePlayer{{Name: "Alice"}}}},
			prevOnline: []string{"Alice"},
			wantKinds:  nil,
		},
		{
			name:      "an unrelated event produces nothing",
			ev:        domain.Event{Name: domain.EventInstanceLog, InstanceID: "main", TS: now, Data: map[string]string{"line": "hi"}},
			wantKinds: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var prev playersSnapshot
			if tc.prevOnline != nil {
				prev = playersSnapshot{}
				for _, n := range tc.prevOnline {
					prev[n] = true
				}
			}
			got := alertsFor(tc.ev, &prev)
			if len(got) != len(tc.wantKinds) {
				t.Fatalf("got %d messages %+v, want %d (%v)", len(got), got, len(tc.wantKinds), tc.wantKinds)
			}
			for i, k := range tc.wantKinds {
				if got[i].Kind != k {
					t.Errorf("message %d kind = %q, want %q", i, got[i].Kind, k)
				}
				if got[i].InstanceID != tc.ev.InstanceID {
					t.Errorf("message %d instance = %q, want %q", i, got[i].InstanceID, tc.ev.InstanceID)
				}
			}
		})
	}
}

func TestAlertsFor_JobFailedTitleAndBody(t *testing.T) {
	ev := domain.Event{Name: domain.EventJobUpdated, InstanceID: "main",
		Data: domain.Job{Type: domain.JobRestore, InstanceID: "main", Status: domain.JobFailed, Error: "zip corrupt"}}
	var prev playersSnapshot
	got := alertsFor(ev, &prev)
	if len(got) != 1 {
		t.Fatalf("expected 1 message, got %+v", got)
	}
	if got[0].Body != "zip corrupt" {
		t.Errorf("expected the job error as body, got %q", got[0].Body)
	}
	if got[0].Title == "" {
		t.Errorf("expected a non-empty title")
	}
}

func TestAlertsFor_PlayersSnapshotPointerIsUpdated(t *testing.T) {
	var prev playersSnapshot
	ev1 := domain.Event{Name: domain.EventInstancePlayers, InstanceID: "main",
		Data: map[string]any{"online": []domain.OnlinePlayer{{Name: "Alice"}}}}
	if got := alertsFor(ev1, &prev); len(got) != 1 || got[0].Kind != domain.AlertPlayerJoin {
		t.Fatalf("expected one join, got %+v", got)
	}
	if !prev["Alice"] {
		t.Fatalf("expected the snapshot to now contain Alice, got %+v", prev)
	}

	// A second identical event produces nothing further (state was updated).
	if got := alertsFor(ev1, &prev); len(got) != 0 {
		t.Fatalf("expected no further alerts once state has converged, got %+v", got)
	}
}
