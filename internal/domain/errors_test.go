package domain

import (
	"strings"
	"testing"
)

func TestValidationError_TextIncludesFieldMessages(t *testing.T) {
	err := Validation([]FieldError{
		{Field: "world", Message: `world "Dedicated" has no save file`},
		{Field: "note", Message: "too long"},
	})
	got := err.Error()
	for _, want := range []string{"validation_failed", `world: world "Dedicated" has no save file`, "note: too long"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, want it to contain %q", got, want)
		}
	}
}

func TestError_TextWithoutFieldsIsUnchanged(t *testing.T) {
	if got := E(CodeNotFound, "world not found").Error(); got != "not_found: world not found" {
		t.Errorf("Error() = %q", got)
	}
}
