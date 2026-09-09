package scheduler

import (
	"strings"

	"github.com/robfig/cron/v3"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// maxNoteLen matches ScheduleInput.note's maxLength in docs/openapi.yaml.
const maxNoteLen = 100

// validateInput checks a ScheduleInput against docs/openapi.yaml's
// ScheduleInput schema plus the cron grammar accepted by parser
// (ARCHITECTURE.md §11: standard 5-field cron, host timezone, plus
// descriptors such as @daily). It never touches the database or checks that
// the instance exists — callers do that separately so the error precedence
// (validation before not_found) stays consistent with the rest of the API.
func validateInput(in domain.ScheduleInput, parser cron.Parser) []domain.FieldError {
	var fs []domain.FieldError

	switch in.Kind {
	case domain.ScheduleRestart, domain.ScheduleBackup, domain.ScheduleUpdate:
	default:
		fs = append(fs, domain.FieldError{Field: "kind", Message: "must be one of restart, backup, update"})
	}

	if strings.TrimSpace(in.Cron) == "" {
		fs = append(fs, domain.FieldError{Field: "cron", Message: "required"})
	} else if _, err := parser.Parse(in.Cron); err != nil {
		fs = append(fs, domain.FieldError{Field: "cron", Message: "invalid cron expression: " + err.Error()})
	}

	if len(in.Note) > maxNoteLen {
		fs = append(fs, domain.FieldError{Field: "note", Message: "must be at most 100 characters"})
	}

	return fs
}
