package auth

import (
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestMapRole(t *testing.T) {
	mapping := map[string]domain.Role{
		"admins":    domain.RoleAdmin,
		"operators": domain.RoleOperator,
		"viewers":   domain.RoleViewer,
	}
	cases := []struct {
		name        string
		groups      []string
		defaultRole string
		wantRole    domain.Role
		wantAllowed bool
	}{
		{"admin group wins", []string{"admins"}, "viewer", domain.RoleAdmin, true},
		{"highest of multiple groups", []string{"viewers", "admins", "operators"}, "viewer", domain.RoleAdmin, true},
		{"unmapped group falls back to default", []string{"nobody"}, "operator", domain.RoleOperator, true},
		{"no groups falls back to default", nil, "viewer", domain.RoleViewer, true},
		{"deny blocks unmapped users", []string{"nobody"}, "deny", "", false},
		{"deny does not block a mapped group", []string{"operators"}, "deny", domain.RoleOperator, true},
		{"empty default treated as viewer", []string{"nobody"}, "", domain.RoleViewer, true},
		{"invalid default role denies", []string{"nobody"}, "bogus", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			role, allowed := mapRole(mapping, c.groups, c.defaultRole)
			if allowed != c.wantAllowed {
				t.Fatalf("allowed = %v, want %v", allowed, c.wantAllowed)
			}
			if allowed && role != c.wantRole {
				t.Fatalf("role = %v, want %v", role, c.wantRole)
			}
		})
	}
}

func TestExtractGroups(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want []string
	}{
		{"json array of strings", []any{"a", "b"}, []string{"a", "b"}},
		{"string slice", []string{"x"}, []string{"x"}},
		{"single string", "solo", []string{"solo"}},
		{"empty string", "", nil},
		{"nil", nil, nil},
		{"unsupported type", 42, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := extractGroups(c.in)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestCandidateAndSanitizeUsername(t *testing.T) {
	if got := candidateUsername(map[string]any{"preferred_username": "Alice.Smith"}); got != "Alice.Smith" {
		t.Fatalf("candidateUsername preferred_username = %q", got)
	}
	if got := candidateUsername(map[string]any{"email": "bob@example.com"}); got != "bob" {
		t.Fatalf("candidateUsername email local part = %q", got)
	}
	if got := candidateUsername(map[string]any{}); got != "user" {
		t.Fatalf("candidateUsername fallback = %q", got)
	}

	if got := sanitizeUsername("Alice.Smith"); got != "alice.smith" {
		t.Fatalf("sanitizeUsername = %q", got)
	}
	if got := sanitizeUsername("Ünïcödé Name!!"); len(got) < 2 {
		t.Fatalf("sanitizeUsername should pad short/invalid results, got %q", got)
	}
	long := ""
	for i := 0; i < 50; i++ {
		long += "a"
	}
	if got := sanitizeUsername(long); len(got) > 32 {
		t.Fatalf("sanitizeUsername should truncate to 32, got len=%d", len(got))
	}
}
