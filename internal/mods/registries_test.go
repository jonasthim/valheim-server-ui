package mods

import (
	"net/http"
	"testing"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// bareClient builds a registry client with no cache/network involvement, for
// the pure aggregator and download-host tests.
func bareClient(id, name string, downloadHosts []string) *Thunderstore {
	return NewThunderstore(id, name, "https://example.test/api/v1/package/", downloadHosts, http.DefaultClient, t0TempDir(), func() time.Duration { return time.Hour }, "test", nil)
}

// t0TempDir returns a throwaway dir path; the client only touches it lazily on
// Refresh, which these tests never call.
func t0TempDir() string { return "/tmp/mods-registries-test-unused" }

func TestRegistries_LookupGetDefaultList(t *testing.T) {
	ts := bareClient(domain.RegistryThunderstoreID, domain.RegistryThunderstoreName, []string{"thunderstore.io"})
	hx := bareClient(domain.RegistryHexiumID, domain.RegistryHexiumName, []string{"hexium.gg"})
	regs := NewRegistries(ts, hx)

	if regs.Default() != ts {
		t.Errorf("default should be the thunderstore registry")
	}
	if got, ok := regs.Lookup(domain.RegistryHexiumID); !ok || got != hx {
		t.Errorf("Lookup(hexium) = %v, %v", got, ok)
	}
	if _, ok := regs.Lookup("manual"); ok {
		t.Errorf("Lookup(manual) should be false")
	}
	if _, ok := regs.Lookup(""); ok {
		t.Errorf("Lookup(\"\") should be false")
	}
	if regs.GetOrDefault("") != ts {
		t.Errorf("GetOrDefault(\"\") should be the default")
	}
	if regs.GetOrDefault("nope") != ts {
		t.Errorf("GetOrDefault(unknown) should be the default")
	}
	if regs.GetOrDefault(domain.RegistryHexiumID) != hx {
		t.Errorf("GetOrDefault(hexium) should be hexium")
	}

	list := regs.List()
	if len(list) != 2 || list[0].ID != domain.RegistryThunderstoreID || list[1].ID != domain.RegistryHexiumID {
		t.Errorf("List() = %+v, want thunderstore then hexium", list)
	}
	if list[0].Name != domain.RegistryThunderstoreName || list[1].Name != domain.RegistryHexiumName {
		t.Errorf("List() names = %+v", list)
	}
}

func TestRegistries_DefaultFallsBackToFirst(t *testing.T) {
	// No thunderstore registry present: the first one is the default.
	hx := bareClient(domain.RegistryHexiumID, domain.RegistryHexiumName, []string{"hexium.gg"})
	regs := NewRegistries(hx)
	if regs.Default() != hx {
		t.Errorf("with no thunderstore, default should be the first registry")
	}
}

func TestRegistries_EmptyIsSafe(t *testing.T) {
	regs := NewRegistries()
	if regs.Default() != nil {
		t.Errorf("empty registries should have a nil default")
	}
	if _, ok := regs.Lookup(domain.RegistryThunderstoreID); ok {
		t.Errorf("empty registries should not resolve any lookup")
	}
	if regs.GetOrDefault("anything") != nil {
		t.Errorf("empty registries GetOrDefault should be nil")
	}
	if len(regs.List()) != 0 {
		t.Errorf("empty registries List should be empty")
	}
}

func TestRegistry_DownloadHostAllowlistIsPerRegistry(t *testing.T) {
	ts := bareClient(domain.RegistryThunderstoreID, domain.RegistryThunderstoreName, []string{"thunderstore.io"})
	hx := bareClient(domain.RegistryHexiumID, domain.RegistryHexiumName, []string{"hexium.gg"})

	cases := []struct {
		name    string
		reg     *Thunderstore
		url     string
		wantErr bool
	}{
		{"thunderstore accepts its own host", ts, "https://thunderstore.io/package/download/a/b/1.0.0/", false},
		{"thunderstore accepts a subdomain", ts, "https://gcdn.thunderstore.io/live/x.zip", false},
		{"thunderstore rejects hexium", ts, "https://cdn.hexium.gg/upload/1/0.1.2.zip", true},
		{"hexium accepts its cdn subdomain", hx, "https://cdn.hexium.gg/upload/1/0.1.2.zip", false},
		{"hexium rejects thunderstore", hx, "https://thunderstore.io/package/download/a/b/1.0.0/", true},
		{"http is rejected", hx, "http://cdn.hexium.gg/upload/1/0.1.2.zip", true},
		{"lookalike host is rejected", hx, "https://hexium.gg.evil.test/x.zip", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.reg.checkDownloadURL(c.url)
			if (err != nil) != c.wantErr {
				t.Errorf("checkDownloadURL(%q) err=%v, wantErr=%v", c.url, err, c.wantErr)
			}
		})
	}
}
