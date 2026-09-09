package auth

import (
	"regexp"
	"strings"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// usernamePattern mirrors SetupRequest/CreateUserRequest in docs/openapi.yaml.
var usernamePattern = regexp.MustCompile(`^[a-z0-9._-]+$`)

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// normalizeUsername lowercases and validates a username, matching the
// SetupRequest/CreateUserRequest schema (2..32 chars, [a-z0-9._-]).
func normalizeUsername(username string) (string, error) {
	u := strings.ToLower(strings.TrimSpace(username))
	if len(u) < 2 || len(u) > 32 || !usernamePattern.MatchString(u) {
		return "", domain.Validation([]domain.FieldError{
			{Field: "username", Message: "must be 2-32 characters of lowercase letters, digits, '.', '_' or '-'"},
		})
	}
	return u, nil
}

func validatePassword(password string) error {
	if len(password) < MinPasswordLength || len(password) > 128 {
		return domain.Validation([]domain.FieldError{
			{Field: "password", Message: "must be 10-128 characters"},
		})
	}
	return nil
}

func validateEmail(email string) error {
	if email == "" {
		return nil
	}
	if len(email) > 254 || !emailPattern.MatchString(email) {
		return domain.Validation([]domain.FieldError{{Field: "email", Message: "invalid email address"}})
	}
	return nil
}

func validateDisplayName(name string) error {
	if len(name) > 64 {
		return domain.Validation([]domain.FieldError{{Field: "display_name", Message: "must be at most 64 characters"}})
	}
	return nil
}
