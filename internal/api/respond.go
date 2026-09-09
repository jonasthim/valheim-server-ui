package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

const maxJSONBody = 1 << 20 // 1 MiB

type errorBody struct {
	Error struct {
		Code    domain.ErrorCode `json:"code"`
		Message string           `json:"message"`
		Details map[string]any   `json:"details,omitempty"`
	} `json:"error"`
}

// WriteJSON writes v with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Warn("write json", "err", err)
	}
}

// WriteError maps any error to the documented error body. Non-domain errors
// become 500 internal without leaking details.
func WriteError(w http.ResponseWriter, err error) {
	de := domain.AsError(err)
	if de.Code == domain.CodeInternal {
		slog.Error("internal error", "err", err)
	}
	var body errorBody
	body.Error.Code = de.Code
	body.Error.Message = de.Message
	if len(de.Fields) > 0 || len(de.Details) > 0 {
		body.Error.Details = map[string]any{}
		for k, v := range de.Details {
			body.Error.Details[k] = v
		}
		if len(de.Fields) > 0 {
			body.Error.Details["fields"] = de.Fields
		}
	}
	WriteJSON(w, de.HTTPStatus(), body)
}

// WriteValidation is a shortcut for field-level validation failures.
func WriteValidation(w http.ResponseWriter, fields ...domain.FieldError) {
	WriteError(w, domain.Validation(fields))
}

// DecodeJSON strictly decodes the request body into dst (unknown fields rejected,
// 1 MiB limit). Returns a validation error suitable for WriteError.
func DecodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return domain.Wrap(domain.CodeValidationFailed, "invalid JSON body: "+err.Error(), err)
	}
	if dec.More() {
		return domain.E(domain.CodeValidationFailed, "unexpected trailing data")
	}
	return nil
}

// DecodeOptionalJSON is DecodeJSON that tolerates an empty body.
func DecodeOptionalJSON(r *http.Request, dst any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	err := DecodeJSON(r, dst)
	if err != nil && errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

// NotImplemented is the placeholder handler used by wave-0 stubs.
func NotImplemented(w http.ResponseWriter, _ *http.Request) {
	WriteJSON(w, http.StatusNotImplemented, map[string]any{
		"error": map[string]any{"code": "not_implemented", "message": "not implemented yet"},
	})
}

// audit is a nil-safe helper for handlers.
//
//nolint:unused // used by handler files as they land
func (d *Deps) audit(r *http.Request, action, instanceID, target string, details map[string]any) {
	if d.Audit != nil {
		d.Audit.Record(r, action, instanceID, target, details)
	}
}
