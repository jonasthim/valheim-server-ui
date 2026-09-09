package mods

import (
	"os"
	"strings"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func loadFixture(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile("testdata/sample.cfg")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return string(data)
}

func entryFor(t *testing.T, entries []domain.ConfigEntry, section, key string) domain.ConfigEntry {
	t.Helper()
	for _, e := range entries {
		if e.Section == section && e.Key == key {
			return e
		}
	}
	t.Fatalf("no entry %s/%s in %+v", section, key, entries)
	return domain.ConfigEntry{}
}

func TestParseCfg_Fixture(t *testing.T) {
	raw := loadFixture(t)
	entries := parseCfg(raw)
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d: %+v", len(entries), entries)
	}

	enabled := entryFor(t, entries, "General", "Enabled")
	if enabled.Value != "true" || enabled.Type != "Boolean" || enabled.DefaultValue != "true" {
		t.Errorf("unexpected Enabled entry: %+v", enabled)
	}
	if enabled.Description != "Whether the plugin is enabled at all." {
		t.Errorf("unexpected description: %q", enabled.Description)
	}

	greeting := entryFor(t, entries, "General", "Greeting")
	wantAccept := []string{"Welcome!", "Hello!", "Hi!"}
	if len(greeting.AcceptableValues) != len(wantAccept) {
		t.Fatalf("unexpected acceptable values: %v", greeting.AcceptableValues)
	}
	for i, v := range wantAccept {
		if greeting.AcceptableValues[i] != v {
			t.Errorf("acceptable_values[%d] = %q, want %q", i, greeting.AcceptableValues[i], v)
		}
	}

	rate := entryFor(t, entries, "Balance", "DropRateMultiplier")
	if rate.Value != "1.5" || rate.Type != "Single" {
		t.Errorf("unexpected DropRateMultiplier: %+v", rate)
	}
	if rate.Range == nil || rate.Range.Min != "0.1" || rate.Range.Max != "5" {
		t.Fatalf("unexpected range: %+v", rate.Range)
	}

	admins := entryFor(t, entries, "Balance", "MaxConcurrentAdmins")
	if admins.Range == nil || admins.Range.Min != "1" || admins.Range.Max != "10" {
		t.Fatalf("unexpected range: %+v", admins.Range)
	}

	flag := entryFor(t, entries, "Balance", "UndocumentedFlag")
	if flag.Value != "false" || flag.Description != "" || flag.Type != "" {
		t.Errorf("expected an undecorated entry, got %+v", flag)
	}
}

func TestApplyCfgValues_PreservesCommentsByteForByte(t *testing.T) {
	raw := loadFixture(t)
	updated, err := applyCfgValues(raw, []domain.ConfigValueUpdate{
		{Section: "General", Key: "Enabled", Value: "false"},
		{Section: "Balance", Key: "DropRateMultiplier", Value: "2.25"},
	})
	if err != nil {
		t.Fatalf("applyCfgValues: %v", err)
	}

	oldLines := strings.Split(raw, "\n")
	newLines := strings.Split(updated, "\n")
	if len(oldLines) != len(newLines) {
		t.Fatalf("line count changed: %d -> %d", len(oldLines), len(newLines))
	}
	changed := map[int]bool{}
	for i := range oldLines {
		if oldLines[i] != newLines[i] {
			changed[i] = true
		}
	}
	if len(changed) != 2 {
		t.Fatalf("expected exactly 2 changed lines, got %d: %v", len(changed), changed)
	}
	if !strings.Contains(updated, "Enabled = false") {
		t.Error("expected Enabled = false in the updated content")
	}
	if !strings.Contains(updated, "DropRateMultiplier = 2.25") {
		t.Error("expected DropRateMultiplier = 2.25 in the updated content")
	}
	// Every comment line must be byte-for-byte identical.
	for i, l := range oldLines {
		if strings.HasPrefix(strings.TrimSpace(l), "#") && l != newLines[i] {
			t.Errorf("comment line %d changed:\n old: %q\n new: %q", i, l, newLines[i])
		}
	}

	// The new content must still parse back to the expected values.
	entries := parseCfg(updated)
	if entryFor(t, entries, "General", "Enabled").Value != "false" {
		t.Error("Enabled did not round-trip to false")
	}
}

func TestApplyCfgValues_UnknownKeyIsValidationError(t *testing.T) {
	raw := loadFixture(t)
	_, err := applyCfgValues(raw, []domain.ConfigValueUpdate{
		{Section: "General", Key: "DoesNotExist", Value: "x"},
	})
	if err == nil {
		t.Fatal("expected a validation error")
	}
	de := domain.AsError(err)
	if de.Code != domain.CodeValidationFailed {
		t.Errorf("expected validation_failed, got %v", de.Code)
	}
	if len(de.Fields) != 1 || de.Fields[0].Field != "values[0]" {
		t.Errorf("unexpected fields: %+v", de.Fields)
	}
}

func TestApplyCfgValues_UnknownSectionIsValidationError(t *testing.T) {
	raw := loadFixture(t)
	_, err := applyCfgValues(raw, []domain.ConfigValueUpdate{
		{Section: "NoSuchSection", Key: "Enabled", Value: "x"},
	})
	de := domain.AsError(err)
	if de.Code != domain.CodeValidationFailed {
		t.Errorf("expected validation_failed, got %v", de.Code)
	}
}

func TestApplyCfgValues_PreservesCRLF(t *testing.T) {
	raw := "[General]\r\n## desc\r\nKey = 1\r\n"
	updated, err := applyCfgValues(raw, []domain.ConfigValueUpdate{{Section: "General", Key: "Key", Value: "2"}})
	if err != nil {
		t.Fatalf("applyCfgValues: %v", err)
	}
	want := "[General]\r\n## desc\r\nKey = 2\r\n"
	if updated != want {
		t.Errorf("got %q, want %q", updated, want)
	}
}

func TestConfigFileName_Validation(t *testing.T) {
	cases := map[string]bool{
		"corelib.cfg":     true,
		"":                false,
		"../corelib.cfg":  false,
		"sub/corelib.cfg": false,
		"corelib.txt":     false,
	}
	for name, ok := range cases {
		err := configFileName(name)
		if (err == nil) != ok {
			t.Errorf("configFileName(%q): err=%v, want ok=%v", name, err, ok)
		}
	}
}

func TestListReadWriteConfigFile(t *testing.T) {
	paths := domain.InstancePaths{Server: t.TempDir()}
	dir := configDir(paths)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/corelib.cfg", []byte(loadFixture(t)), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := listConfigFiles(paths)
	if err != nil {
		t.Fatalf("listConfigFiles: %v", err)
	}
	if len(files) != 1 || files[0].Name != "corelib.cfg" {
		t.Fatalf("unexpected files: %+v", files)
	}

	cf, err := readConfigFile(paths, "corelib.cfg")
	if err != nil {
		t.Fatalf("readConfigFile: %v", err)
	}
	if len(cf.Entries) != 5 {
		t.Fatalf("expected 5 parsed entries, got %d", len(cf.Entries))
	}

	updated, err := writeConfigFile(paths, "corelib.cfg", domain.ConfigFileUpdate{
		Values: []domain.ConfigValueUpdate{{Section: "General", Key: "Enabled", Value: "false"}},
	})
	if err != nil {
		t.Fatalf("writeConfigFile (values): %v", err)
	}
	if entryFor(t, updated.Entries, "General", "Enabled").Value != "false" {
		t.Error("expected Enabled updated to false")
	}

	rawReplacement := "[General]\nEnabled = maybe\n"
	updated, err = writeConfigFile(paths, "corelib.cfg", domain.ConfigFileUpdate{Raw: &rawReplacement})
	if err != nil {
		t.Fatalf("writeConfigFile (raw): %v", err)
	}
	if updated.Raw != rawReplacement {
		t.Errorf("expected raw mode to replace the file wholesale, got %q", updated.Raw)
	}
}

// BepInEx's ConfigFile.Save orders sections as plain strings, so mods that
// number their sections ("1 - …", "2 - …", "10 - …") come out of the file as
// 1, 10, 2. The editor should show them in natural order while keeping each
// section's keys in file order.
func TestParseCfg_SectionsInNaturalOrder(t *testing.T) {
	raw := strings.Join([]string{
		"[1 - Server & Sync]",
		"Lock Configuration = On",
		"",
		"[10 - Favoriting]",
		"Favoriting Modifier Key = LeftAlt",
		"Zebra = 1",
		"Alpha = 2",
		"",
		"[2 - Chests]",
		"Range = 5",
		"",
		"[General]",
		"Enabled = true",
		"",
	}, "\n")
	entries := parseCfg(raw)
	var got []string
	for _, e := range entries {
		got = append(got, e.Section+"/"+e.Key)
	}
	want := []string{
		"1 - Server & Sync/Lock Configuration",
		"2 - Chests/Range",
		"10 - Favoriting/Favoriting Modifier Key",
		"10 - Favoriting/Zebra",
		"10 - Favoriting/Alpha",
		"General/Enabled",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("section order\n got %v\nwant %v", got, want)
	}
}
