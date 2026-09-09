package scheduler

import (
	"context"
	"fmt"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Service satisfies api.ScheduleService structurally (verified where it is
// wired to api.Deps.Schedules in cmd/valheim-ui). It deliberately does not
// import internal/api itself: internal/api's own internal test files import
// internal/instance (WP-02, for a real Service in handler tests) and this
// package (to build a real scheduler.Service for schedules_handlers_test.go),
// so this package importing internal/api back would be an import cycle for
// those test files (see the same note in internal/jobs/runner.go).

// List implements api.ScheduleService.
func (s *Service) List(ctx context.Context, instanceID string) ([]domain.Schedule, error) {
	if err := s.checkInstanceExists(ctx, instanceID); err != nil {
		return nil, err
	}
	rows, err := s.listScheduleRows(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Schedule, 0, len(rows))
	for _, r := range rows {
		out = append(out, s.toDomain(r))
	}
	return out, nil
}

// Create implements api.ScheduleService.
func (s *Service) Create(ctx context.Context, instanceID string, in domain.ScheduleInput) (*domain.Schedule, error) {
	if fs := validateInput(in, s.parser); len(fs) > 0 {
		return nil, domain.Validation(fs)
	}
	if err := s.checkInstanceExists(ctx, instanceID); err != nil {
		return nil, err
	}

	now := s.now().UTC()
	row := scheduleRow{
		InstanceID:    instanceID,
		Kind:          string(in.Kind),
		CronExpr:      in.Cron,
		Enabled:       in.Enabled,
		OnlyWhenEmpty: in.OnlyWhenEmpty,
		Note:          in.Note,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	id, err := s.insertScheduleRow(ctx, row)
	if err != nil {
		return nil, err
	}
	row.ID = id
	s.reload(ctx)

	sc := s.toDomain(row)
	return &sc, nil
}

// Update implements api.ScheduleService.
func (s *Service) Update(ctx context.Context, instanceID string, id int64, in domain.ScheduleInput) (*domain.Schedule, error) {
	if fs := validateInput(in, s.parser); len(fs) > 0 {
		return nil, domain.Validation(fs)
	}
	if err := s.checkInstanceExists(ctx, instanceID); err != nil {
		return nil, err
	}
	row, err := s.getOwnedScheduleRow(ctx, instanceID, id)
	if err != nil {
		return nil, err
	}

	row.Kind = string(in.Kind)
	row.CronExpr = in.Cron
	row.Enabled = in.Enabled
	row.OnlyWhenEmpty = in.OnlyWhenEmpty
	row.Note = in.Note
	row.UpdatedAt = s.now().UTC()
	if err := s.updateScheduleRow(ctx, row); err != nil {
		return nil, err
	}
	s.reload(ctx)

	sc := s.toDomain(row)
	return &sc, nil
}

// Delete implements api.ScheduleService.
func (s *Service) Delete(ctx context.Context, instanceID string, id int64) error {
	if err := s.checkInstanceExists(ctx, instanceID); err != nil {
		return err
	}
	if _, err := s.getOwnedScheduleRow(ctx, instanceID, id); err != nil {
		return err
	}
	if err := s.deleteScheduleRow(ctx, id); err != nil {
		return err
	}
	s.reload(ctx)
	return nil
}

// RunNow implements api.ScheduleService: it runs the schedule's action
// immediately and returns the enqueued job, or a conflict error describing
// why the action was skipped.
func (s *Service) RunNow(ctx context.Context, instanceID string, id int64, requestedBy string) (*domain.Job, error) {
	if err := s.checkInstanceExists(ctx, instanceID); err != nil {
		return nil, err
	}
	row, err := s.getOwnedScheduleRow(ctx, instanceID, id)
	if err != nil {
		return nil, err
	}

	job, skipReason, err := s.runAndRecord(ctx, row, requestedBy)
	if err != nil {
		return nil, err
	}
	if skipReason != "" {
		return nil, domain.Ef(domain.CodeConflict, "skipped: %s", skipReason)
	}
	return job, nil
}

// checkInstanceExists returns domain.NotFound("instance") when instanceID
// does not exist, mirroring the other resource services (see
// internal/players/service.go).
func (s *Service) checkInstanceExists(ctx context.Context, instanceID string) error {
	ok, err := s.inst.Exists(ctx, instanceID)
	if err != nil {
		return fmt.Errorf("check instance %s: %w", instanceID, err)
	}
	if !ok {
		return domain.NotFound("instance")
	}
	return nil
}

// getOwnedScheduleRow fetches a schedule and verifies it belongs to
// instanceID, returning domain.NotFound("schedule") otherwise so a schedule
// id from a different instance never leaks through the URL for this one.
func (s *Service) getOwnedScheduleRow(ctx context.Context, instanceID string, id int64) (scheduleRow, error) {
	row, err := s.getScheduleRow(ctx, id)
	if err != nil {
		return scheduleRow{}, err
	}
	if row.InstanceID != instanceID {
		return scheduleRow{}, domain.NotFound("schedule")
	}
	return row, nil
}

// toDomain renders a scheduleRow as the API's domain.Schedule, computing
// NextRunAt from the cron expression when the schedule is enabled.
func (s *Service) toDomain(r scheduleRow) domain.Schedule {
	sc := domain.Schedule{
		ScheduleInput: domain.ScheduleInput{
			Kind:          domain.ScheduleKind(r.Kind),
			Cron:          r.CronExpr,
			Enabled:       r.Enabled,
			OnlyWhenEmpty: r.OnlyWhenEmpty,
			Note:          r.Note,
		},
		ID:         r.ID,
		InstanceID: r.InstanceID,
		LastRunAt:  r.LastRunAt,
		LastResult: r.LastResult,
		LastJobID:  r.LastJobID,
	}
	if r.Enabled {
		if parsed, err := s.parser.Parse(r.CronExpr); err == nil {
			next := parsed.Next(s.now().In(s.loc))
			sc.NextRunAt = &next
		}
	}
	return sc
}
