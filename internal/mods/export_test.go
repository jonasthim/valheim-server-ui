package mods

import (
	"archive/zip"
	"bytes"
	"io"
	"reflect"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// readZipEntry reads one named entry from zip bytes, failing the test if it
// is missing.
func readZipEntry(t *testing.T, zipBytes []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", name, err)
		}
		defer func() { _ = rc.Close() }()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read entry %s: %v", name, err)
		}
		return data
	}
	t.Fatalf("zip missing entry %s", name)
	return nil
}

func zipFileNames(t *testing.T, zipBytes []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names
}

func TestBuildR2Profile(t *testing.T) {
	mods := []domain.Mod{
		{Source: domain.ModSourceThunderstore, Owner: "Alice", Name: "CoreLib", Version: "1.2.3", Enabled: true},
		{Source: domain.ModSourceManual, Owner: "Bob", Name: "LocalMod", Version: "1.0.0", Enabled: true},
		{Source: domain.ModSourceThunderstore, Owner: "Carol", Name: "BadVersion", Version: "abc", Enabled: false},
	}
	configs := []domain.ConfigFile{
		{Name: "foo.cfg", Raw: "[General]\nEnabled = true\n"},
	}

	zipBytes, skipped, err := BuildR2Profile("main", mods, configs)
	if err != nil {
		t.Fatalf("BuildR2Profile: %v", err)
	}

	gotNames := zipFileNames(t, zipBytes)
	wantNames := []string{"export.r2x", "config/foo.cfg"}
	if len(gotNames) != len(wantNames) {
		t.Fatalf("expected exactly %v, got %v", wantNames, gotNames)
	}
	for _, n := range wantNames {
		found := false
		for _, g := range gotNames {
			if g == n {
				found = true
			}
		}
		if !found {
			t.Errorf("zip missing entry %s (got %v)", n, gotNames)
		}
	}

	var profile r2xProfile
	if err := yaml.Unmarshal(readZipEntry(t, zipBytes, "export.r2x"), &profile); err != nil {
		t.Fatalf("unmarshal export.r2x: %v", err)
	}
	if profile.ProfileName != "main" {
		t.Errorf("profileName = %q, want %q", profile.ProfileName, "main")
	}
	if len(profile.Mods) != 1 {
		t.Fatalf("expected 1 mod in profile, got %+v", profile.Mods)
	}
	got := profile.Mods[0]
	want := r2xMod{Name: "Alice-CoreLib", Version: r2xVersion{Major: 1, Minor: 2, Patch: 3}, Enabled: true}
	if got != want {
		t.Errorf("mod entry = %+v, want %+v", got, want)
	}

	if cfgData := readZipEntry(t, zipBytes, "config/foo.cfg"); string(cfgData) != configs[0].Raw {
		t.Errorf("config/foo.cfg = %q, want %q", cfgData, configs[0].Raw)
	}

	wantSkipped := []string{"Bob-LocalMod", "Carol-BadVersion"}
	if !reflect.DeepEqual(skipped, wantSkipped) {
		t.Errorf("skipped = %v, want %v", skipped, wantSkipped)
	}
}

func TestBuildR2Profile_NoThunderstoreMods(t *testing.T) {
	mods := []domain.Mod{
		{Source: domain.ModSourceManual, Owner: "Bob", Name: "LocalMod", Version: "1.0.0", Enabled: true},
		{Source: domain.ModSourceBundled, Owner: "jonasthim", Name: "valheimui_agent", Version: "1.0.0", Enabled: true},
	}

	zipBytes, skipped, err := BuildR2Profile("main", mods, nil)
	if err != nil {
		t.Fatalf("BuildR2Profile: %v", err)
	}

	var profile r2xProfile
	if err := yaml.Unmarshal(readZipEntry(t, zipBytes, "export.r2x"), &profile); err != nil {
		t.Fatalf("unmarshal export.r2x: %v", err)
	}
	if len(profile.Mods) != 0 {
		t.Errorf("expected zero mods, got %+v", profile.Mods)
	}

	wantSkipped := []string{"Bob-LocalMod", "jonasthim-valheimui_agent"}
	if !reflect.DeepEqual(skipped, wantSkipped) {
		t.Errorf("skipped = %v, want %v", skipped, wantSkipped)
	}
}
