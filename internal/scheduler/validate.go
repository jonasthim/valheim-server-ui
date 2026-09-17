package scheduler

import (
	"strings"

	"github.com/robfig/cron/v3"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// maxNoteLen matches ScheduleInput.note's maxLength in docs/openapi.yaml.
const maxNoteLen = 100

// maxMessageLen matches ScheduleInput.message's maxLength in docs/openapi.yaml
// (an "announce" schedule's broadcast text).
const maxMessageLen = 500

// maxLeadSeconds matches ScheduleInput.lead_seconds' maximum in
// docs/openapi.yaml (a "restart" schedule's own warning lead).
const maxLeadSeconds = 3600

// validateInput checks a ScheduleInput against docs/openapi.yaml's
// ScheduleInput schema plus the cron grammar accepted by parser
// (ARCHITECTURE.md §11: standard 5-field cron, host timezone, plus
// descriptors such as @daily). It never touches the database or checks that
// the instance exists — callers do that separately so the error precedence
// (validation before not_found) stays consistent with the rest of the API.
func validateInput(in domain.ScheduleInput, parser cron.Parser) []domain.FieldError {
	var fs []domain.FieldError

	switch in.Kind {
	case domain.ScheduleRestart, domain.ScheduleBackup, domain.ScheduleUpdate,
		domain.ScheduleAnnounce, domain.ScheduleCommand, domain.ScheduleSave:
	default:
		fs = append(fs, domain.FieldError{Field: "kind", Message: "must be one of restart, backup, update, announce, command, save"})
	}

	if strings.TrimSpace(in.Cron) == "" {
		fs = append(fs, domain.FieldError{Field: "cron", Message: "required"})
	} else if _, err := parser.Parse(in.Cron); err != nil {
		fs = append(fs, domain.FieldError{Field: "cron", Message: "invalid cron expression: " + err.Error()})
	}

	if len(in.Note) > maxNoteLen {
		fs = append(fs, domain.FieldError{Field: "note", Message: "must be at most 100 characters"})
	}

	fs = append(fs, validateKindPayload(in)...)

	return fs
}

// validateKindPayload checks the fields specific to each schedule kind:
// announce needs a non-empty message, command needs a valid agent command,
// save needs nothing, restart's lead_seconds must be in range, and no kind
// but its own may carry message/command.
func validateKindPayload(in domain.ScheduleInput) []domain.FieldError {
	var fs []domain.FieldError

	switch in.Kind {
	case domain.ScheduleAnnounce:
		if strings.TrimSpace(in.Message) == "" {
			fs = append(fs, domain.FieldError{Field: "message", Message: "required"})
		} else if len(in.Message) > maxMessageLen {
			fs = append(fs, domain.FieldError{Field: "message", Message: "must be at most 500 characters"})
		}
	case domain.ScheduleCommand:
		if in.Command == nil {
			fs = append(fs, domain.FieldError{Field: "command", Message: "required"})
		} else {
			// ValidateAgentCommand checks a verb's arguments, not the verb
			// itself (the API handler does that separately), so an unknown
			// command must be rejected here or it would only fail at run time.
			known := false
			for _, c := range domain.AgentCommands {
				if c == in.Command.Command {
					known = true
					break
				}
			}
			if !known {
				fs = append(fs, domain.FieldError{Field: "command.command", Message: "must be one of " + strings.Join(domain.AgentCommands, ", ")})
			}
			for _, cf := range domain.ValidateAgentCommand(*in.Command) {
				fs = append(fs, domain.FieldError{Field: "command." + cf.Field, Message: cf.Message})
			}
		}
	case domain.ScheduleRestart:
		if in.LeadSeconds < 0 || in.LeadSeconds > maxLeadSeconds {
			fs = append(fs, domain.FieldError{Field: "lead_seconds", Message: "must be between 0 and 3600"})
		}
	}

	if in.Kind != domain.ScheduleAnnounce && in.Message != "" {
		fs = append(fs, domain.FieldError{Field: "message", Message: "only valid for an announce schedule"})
	}
	if in.Kind != domain.ScheduleCommand && in.Command != nil {
		fs = append(fs, domain.FieldError{Field: "command", Message: "only valid for a command schedule"})
	}

	return fs
}
