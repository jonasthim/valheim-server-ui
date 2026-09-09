package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
)

func newTestService(t *testing.T, opts ...Option) (*Service, *fakePlayers) {
	t.Helper()
	sqldb := newTestDB(t)
	inst, _ := newTestInstanceService(t, sqldb)
	createTestInstance(t, inst, "main")
	runner := jobs.New(sqldb, nil, t.TempDir(), newTestLogger())
	players := newFakePlayers()
	hooks := Hooks{
		Backup: func(_ context.Context, _, _ string) (*domain.Job, error) {
			return nil, domain.E(domain.CodeInternal, "backup hook not configured for this test")
		},
		Update: func(_ context.Context, _, _ string, _ bool) (*domain.Job, error) {
			return nil, domain.E(domain.CodeInternal, "update hook not configured for this test")
		},
		UpdateAvailable: func(_ context.Context, _ string) (bool, error) { return false, nil },
	}
	svc := New(sqldb, inst, runner, players, hooks, newTestLogger(), opts...)
	return svc, players
}

func TestCreate_ValidatesBeforeCheckingInstance(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Create(context.Background(), "does-not-exist", domain.ScheduleInput{Kind: "bogus", Cron: ""})
	de := requireDomainError(t, err)
	if de.Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation_failed, got %v", de.Code)
	}
}

func TestCreate_UnknownInstance(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.Create(context.Background(), "does-not-exist", domain.ScheduleInput{Kind: domain.ScheduleBackup, Cron: "@daily", Enabled: true})
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Fatalf("expected not_found, got %v", de.Code)
	}
}

func TestCreate_Success(t *testing.T) {
	svc, _ := newTestService(t)
	sc, err := svc.Create(context.Background(), "main", domain.ScheduleInput{
		Kind: domain.ScheduleBackup, Cron: "0 3 * * *", Enabled: true, OnlyWhenEmpty: true, Note: "nightly",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sc.ID == 0 || sc.InstanceID != "main" || sc.Kind != domain.ScheduleBackup || sc.Cron != "0 3 * * *" {
		t.Fatalf("unexpected schedule: %+v", sc)
	}
	if sc.NextRunAt == nil {
		t.Fatal("expected NextRunAt to be set for an enabled schedule")
	}

	list, err := svc.List(context.Background(), "main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != sc.ID {
		t.Fatalf("expected the created schedule in List, got %+v", list)
	}
}

func TestNextRunAt_KnownExpressions(t *testing.T) {
	fixedNow := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	svc, _ := newTestService(t, WithClock(func() time.Time { return fixedNow }), WithLocation(time.UTC))

	cases := []struct {
		cron string
		want time.Time
	}{
		{cron: "0 0 * * *", want: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)},    // @daily, next midnight
		{cron: "0 3 * * *", want: time.Date(2024, 1, 2, 3, 0, 0, 0, time.UTC)},    // next 3am
		{cron: "* * * * *", want: time.Date(2024, 1, 1, 12, 1, 0, 0, time.UTC)},   // next minute
		{cron: "0 */6 * * *", want: time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC)}, // every 6h
	}

	for i, tc := range cases {
		sc, err := svc.Create(context.Background(), "main", domain.ScheduleInput{
			Kind: domain.ScheduleBackup, Cron: tc.cron, Enabled: true,
		})
		if err != nil {
			t.Fatalf("case %d: Create(%q): %v", i, tc.cron, err)
		}
		if sc.NextRunAt == nil {
			t.Fatalf("case %d: NextRunAt nil for %q", i, tc.cron)
		}
		if !sc.NextRunAt.Equal(tc.want) {
			t.Errorf("case %d: cron %q: NextRunAt = %v, want %v", i, tc.cron, sc.NextRunAt.UTC(), tc.want)
		}
	}
}

func TestNextRunAt_NilWhenDisabled(t *testing.T) {
	fixedNow := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	svc, _ := newTestService(t, WithClock(func() time.Time { return fixedNow }), WithLocation(time.UTC))
	sc, err := svc.Create(context.Background(), "main", domain.ScheduleInput{
		Kind: domain.ScheduleBackup, Cron: "0 3 * * *", Enabled: false,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if sc.NextRunAt != nil {
		t.Errorf("expected nil NextRunAt for a disabled schedule, got %v", sc.NextRunAt)
	}
}

func TestUpdate_CrossInstanceIsNotFound(t *testing.T) {
	svc, _ := newTestService(t)
	inst, _ := newTestInstanceService(t, svc.db)
	createTestInstance(t, inst, "other")

	sc, err := svc.Create(context.Background(), "main", domain.ScheduleInput{
		Kind: domain.ScheduleBackup, Cron: "@daily", Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	_, err = svc.Update(context.Background(), "other", sc.ID, domain.ScheduleInput{
		Kind: domain.ScheduleBackup, Cron: "@hourly", Enabled: true,
	})
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Fatalf("expected not_found for a schedule id belonging to a different instance, got %v", de.Code)
	}
}

func TestUpdate_Success(t *testing.T) {
	svc, _ := newTestService(t)
	sc, err := svc.Create(context.Background(), "main", domain.ScheduleInput{
		Kind: domain.ScheduleBackup, Cron: "@daily", Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := svc.Update(context.Background(), "main", sc.ID, domain.ScheduleInput{
		Kind: domain.ScheduleRestart, Cron: "@hourly", Enabled: false, OnlyWhenEmpty: true, Note: "changed",
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Kind != domain.ScheduleRestart || updated.Cron != "@hourly" || updated.Enabled || updated.Note != "changed" {
		t.Fatalf("unexpected updated schedule: %+v", updated)
	}
}

func TestDelete(t *testing.T) {
	svc, _ := newTestService(t)
	sc, err := svc.Create(context.Background(), "main", domain.ScheduleInput{
		Kind: domain.ScheduleBackup, Cron: "@daily", Enabled: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(context.Background(), "main", sc.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	list, err := svc.List(context.Background(), "main")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected no schedules after Delete, got %+v", list)
	}

	// Deleting again is not_found.
	err = svc.Delete(context.Background(), "main", sc.ID)
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Fatalf("expected not_found on double delete, got %v", de.Code)
	}
}

func TestList_UnknownInstance(t *testing.T) {
	svc, _ := newTestService(t)
	_, err := svc.List(context.Background(), "does-not-exist")
	de := requireDomainError(t, err)
	if de.Code != domain.CodeNotFound {
		t.Fatalf("expected not_found, got %v", de.Code)
	}
}
