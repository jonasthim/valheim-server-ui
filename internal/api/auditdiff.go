package api

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
)

// auditChange is one field-level difference recorded under details.changes
// of an audit entry, so the log shows what an update actually changed
// instead of a dump of the whole object. Path is a dotted JSON path
// ("config.modifiers.portals"); a missing From or To means "unset".
type auditChange struct {
	Path string `json:"path"`
	From any    `json:"from,omitempty"`
	To   any    `json:"to,omitempty"`
}

// maskedValue replaces secret values in changes; the entry still records
// that the field changed.
const maskedValue = "••••••"

// sensitiveKeys are JSON keys whose values must never reach the audit log.
var sensitiveKeys = map[string]bool{"password": true, "client_secret": true, "secret": true, "token": true}

// auditDiff compares two values through their JSON form and returns the
// leaf-level differences. Objects are descended key by key; arrays and
// scalars are compared as a whole. Sensitive keys are masked. The result is
// sorted by path and never nil (an empty slice means "nothing changed").
func auditDiff(before, after any) []auditChange {
	out := []auditChange{}
	walkDiff("", toJSONValue(before), toJSONValue(after), &out)
	for i := range out {
		if sensitiveKeys[lastSegment(out[i].Path)] {
			if out[i].From != nil {
				out[i].From = maskedValue
			}
			if out[i].To != nil {
				out[i].To = maskedValue
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func walkDiff(path string, before, after any, out *[]auditChange) {
	bm, bok := before.(map[string]any)
	am, aok := after.(map[string]any)
	// Descend when at least one side is an object and the other is an object
	// or absent, so a newly set (or fully cleared) object still reports its
	// leaves rather than one opaque change.
	if (bok || aok) && (bok || before == nil) && (aok || after == nil) {
		if bm == nil {
			bm = map[string]any{}
		}
		if am == nil {
			am = map[string]any{}
		}
		keys := map[string]struct{}{}
		for k := range bm {
			keys[k] = struct{}{}
		}
		for k := range am {
			keys[k] = struct{}{}
		}
		for k := range keys {
			walkDiff(joinPath(path, k), bm[k], am[k], out)
		}
		return
	}
	if !reflect.DeepEqual(before, after) {
		*out = append(*out, auditChange{Path: path, From: before, To: after})
	}
}

func toJSONValue(v any) any {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil
	}
	return out
}

func joinPath(base, key string) string {
	if base == "" {
		return key
	}
	return base + "." + key
}

func lastSegment(path string) string {
	if i := strings.LastIndexByte(path, '.'); i >= 0 {
		return path[i+1:]
	}
	return path
}
