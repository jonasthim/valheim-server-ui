// Package domain holds the types shared by every other package: entities,
// enums, validation rules and the error vocabulary exposed by the HTTP API.
// It must not import any other internal package.
package domain

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// ErrorCode is the machine-readable error code returned in API error bodies.
// Keep in sync with docs/openapi.yaml → ErrorResponse.
type ErrorCode string

const (
	CodeUnauthorized         ErrorCode = "unauthorized"
	CodeForbidden            ErrorCode = "forbidden"
	CodeNotFound             ErrorCode = "not_found"
	CodeValidationFailed     ErrorCode = "validation_failed"
	CodeConflict             ErrorCode = "conflict"
	CodeInstanceRunning      ErrorCode = "instance_running"
	CodeInstanceNotInstalled ErrorCode = "instance_not_installed"
	CodeInstanceBusy         ErrorCode = "instance_busy"
	CodePortInUse            ErrorCode = "port_in_use"
	CodeLastAdmin            ErrorCode = "last_admin"
	CodeInvalidCredentials   ErrorCode = "invalid_credentials" //nolint:gosec // error code, not a credential
	CodeAccountLocked        ErrorCode = "account_locked"
	CodeAccountDisabled      ErrorCode = "account_disabled"
	CodeSetupDone            ErrorCode = "setup_done"
	CodeOIDCDisabled         ErrorCode = "oidc_disabled"
	CodeOIDCError            ErrorCode = "oidc_error"
	CodeSteamCMDMissing      ErrorCode = "steamcmd_missing"
	CodeBepInExMissing       ErrorCode = "bepinex_missing"
	CodePackageNotFound      ErrorCode = "package_not_found"
	CodeUpstreamError        ErrorCode = "upstream_error"
	CodeInternal             ErrorCode = "internal"
)

// FieldError describes a single invalid input field.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Error is the error type services return; the API layer maps it to a status
// code and JSON body. Wrap lower-level errors with %w so callers can errors.As.
type Error struct {
	Code    ErrorCode
	Message string
	Fields  []FieldError
	Details map[string]any
	Cause   error
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString(string(e.Code))
	b.WriteString(": ")
	b.WriteString(e.Message)
	if e.Cause != nil {
		b.WriteString(": ")
		b.WriteString(e.Cause.Error())
	}
	// Field messages are what a human needs to act on ("world "X" has no
	// save file"); without them a failed job only says "validation failed".
	for _, f := range e.Fields {
		b.WriteString("; ")
		b.WriteString(f.Field)
		b.WriteString(": ")
		b.WriteString(f.Message)
	}
	return b.String()
}

func (e *Error) Unwrap() error { return e.Cause }

// HTTPStatus maps an error code to the HTTP status used by the API.
func (e *Error) HTTPStatus() int {
	switch e.Code {
	case CodeUnauthorized, CodeInvalidCredentials:
		return http.StatusUnauthorized
	case CodeForbidden, CodeAccountDisabled:
		return http.StatusForbidden
	case CodeNotFound, CodeSetupDone, CodeOIDCDisabled, CodePackageNotFound:
		return http.StatusNotFound
	case CodeValidationFailed:
		return http.StatusUnprocessableEntity
	case CodeConflict, CodeInstanceRunning, CodeInstanceNotInstalled, CodeInstanceBusy,
		CodePortInUse, CodeLastAdmin, CodeBepInExMissing:
		return http.StatusConflict
	case CodeAccountLocked:
		return http.StatusLocked
	case CodeSteamCMDMissing:
		return http.StatusFailedDependency
	case CodeUpstreamError:
		return http.StatusBadGateway
	default:
		return http.StatusInternalServerError
	}
}

// E constructs a domain error.
func E(code ErrorCode, msg string) *Error { return &Error{Code: code, Message: msg} }

// Ef constructs a domain error with a formatted message.
func Ef(code ErrorCode, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Wrap attaches a cause to a domain error.
func Wrap(code ErrorCode, msg string, cause error) *Error {
	return &Error{Code: code, Message: msg, Cause: cause}
}

// NotFound is a convenience for the most common error.
func NotFound(what string) *Error { return Ef(CodeNotFound, "%s not found", what) }

// Validation builds a validation error from field errors. Returns nil when the
// list is empty so callers can write `return domain.Validation(fields)`.
func Validation(fields []FieldError) error {
	if len(fields) == 0 {
		return nil
	}
	return &Error{Code: CodeValidationFailed, Message: "validation failed", Fields: fields}
}

// AsError extracts a *Error from err, or wraps it as an internal error.
func AsError(err error) *Error {
	var de *Error
	if errors.As(err, &de) {
		return de
	}
	return &Error{Code: CodeInternal, Message: "internal error", Cause: err}
}
