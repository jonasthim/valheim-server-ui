package mods

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestConfigFormat_AndFileName(t *testing.T) {
	cases := []struct {
		name   string
		format domain.ConfigFileFormat
		ok     bool
	}{
		{"a.cfg", domain.ConfigFormatCfg, true},
		{"A.CFG", domain.ConfigFormatCfg, true},
		{"a.yml", domain.ConfigFormatYAML, true},
		{"a.YAML", domain.ConfigFormatYAML, true},
		{"a.Yml", domain.ConfigFormatYAML, true},
		{"a.json", domain.ConfigFormatJSON, true},
		{"a.JSON", domain.ConfigFormatJSON, true},
		{"a.txt", "", false},
		{"a", "", false},
		{".a.yml", "", false},
		{".Azumatt.AzuAutoStore.yml.swp", "", false},
		{"sub/a.yml", "", false},
		{"../a.json", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		err := configFileName(c.name)
		if (err == nil) != c.ok {
			t.Errorf("configFileName(%q): err=%v, want ok=%v", c.name, err, c.ok)
		}
		if c.ok {
			if f, ok := configFormat(c.name); !ok || f != c.format {
				t.Errorf("configFormat(%q) = %q,%v want %q", c.name, f, ok, c.format)
			}
		}
	}
}

func newConfigDirPaths(t *testing.T) (domain.InstancePaths, string) {
	t.Helper()
	paths := domain.InstancePaths{Server: t.TempDir()}
	dir := configDir(paths)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return paths, dir
}

func TestListConfigFiles_MixedFormats(t *testing.T) {
	paths, dir := newConfigDirPaths(t)
	for _, n := range []string{"b.cfg", "a.yml", "c.yaml", "d.JSON", "notes.txt", ".Azumatt.AzuAutoStore.yml.swp", ".hidden.yml"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.yml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "a.yml"), filepath.Join(dir, "link.yml")); err != nil {
		t.Fatal(err)
	}

	files, err := listConfigFiles(paths)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]domain.ConfigFileFormat{}
	var order []string
	for _, f := range files {
		got[f.Name] = f.Format
		order = append(order, f.Name)
	}
	want := map[string]domain.ConfigFileFormat{
		"a.yml": domain.ConfigFormatYAML, "b.cfg": domain.ConfigFormatCfg,
		"c.yaml": domain.ConfigFormatYAML, "d.JSON": domain.ConfigFormatJSON,
	}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for n, f := range want {
		if got[n] != f {
			t.Errorf("%s: format %q, want %q", n, got[n], f)
		}
	}
	if strings.Join(order, ",") != "a.yml,b.cfg,c.yaml,d.JSON" {
		t.Errorf("not sorted by name: %v", order)
	}
}

func loadYAMLFixture(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/Azumatt.AzuAutoStore.yml")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestYAMLFixture_ReadAndByteIdenticalWrite(t *testing.T) {
	paths, dir := newConfigDirPaths(t)
	fixture := loadYAMLFixture(t)
	if !strings.Contains(string(fixture), "#- Food") {
		t.Fatal("fixture lost its commented-out list items")
	}
	name := "Azumatt.AzuAutoStore.yml"
	if err := os.WriteFile(filepath.Join(dir, name), fixture, 0o640); err != nil {
		t.Fatal(err)
	}

	cf, err := readConfigFile(paths, name)
	if err != nil {
		t.Fatalf("readConfigFile: %v", err)
	}
	if cf.Format != domain.ConfigFormatYAML || cf.Raw != string(fixture) {
		t.Errorf("format=%q, raw unchanged=%v", cf.Format, cf.Raw == string(fixture))
	}
	if cf.Entries == nil || len(cf.Entries) != 0 {
		t.Errorf("entries must be empty and non-nil, got %#v", cf.Entries)
	}

	raw := string(fixture)
	if _, err := writeConfigFile(paths, name, domain.ConfigFileUpdate{Raw: &raw}); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(fixture) {
		t.Error("file is not byte-identical after writing the fixture back")
	}
}

func TestWriteConfigFile_YAMLInvalidLeavesFileUntouched(t *testing.T) {
	paths, dir := newConfigDirPaths(t)
	fixture := loadYAMLFixture(t)
	name := "Azumatt.AzuAutoStore.yml"
	if err := os.WriteFile(filepath.Join(dir, name), fixture, 0o640); err != nil {
		t.Fatal(err)
	}
	broken := "groups:\n  Food:\n - a\n    - b\n   bad: [\n"
	_, err := writeConfigFile(paths, name, domain.ConfigFileUpdate{Raw: &broken})
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != domain.CodeValidationFailed {
		t.Fatalf("expected validation error, got %v", err)
	}
	if len(de.Fields) != 1 || de.Fields[0].Field != "raw" || !strings.Contains(de.Fields[0].Message, "line") {
		t.Errorf("unexpected field errors: %+v", de.Fields)
	}
	after, _ := os.ReadFile(filepath.Join(dir, name))
	if string(after) != string(fixture) {
		t.Error("file on disk changed despite a rejected write")
	}
}

func TestWriteConfigFile_YAMLJSONRoundTrips(t *testing.T) {
	cases := []struct{ name, raw string }{
		{"empty.yml", "{}"},
		{"empty-nl.yaml", "{}\n"},
		{"neg.yml", "-5: a\n7: b\n  \n# comment  \n"},
		{"comment-only.yml", "# nothing here  \n"},
		{"multi.yml", "a: 1\n---\nb: 2\n"},
		{"crlf.yml", "a: 1\r\nb: 2\r\n"},
		{"ok.json", "{\"a\": [1, 2],\n \"b\": null}  "},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			paths, dir := newConfigDirPaths(t)
			raw := c.raw
			cf, err := writeConfigFile(paths, c.name, domain.ConfigFileUpdate{Raw: &raw})
			if err != nil {
				t.Fatalf("write: %v", err)
			}
			if cf.Raw != c.raw {
				t.Errorf("raw returned %q, want %q", cf.Raw, c.raw)
			}
			disk, _ := os.ReadFile(filepath.Join(dir, c.name))
			if string(disk) != c.raw {
				t.Errorf("disk %q, want %q", disk, c.raw)
			}
		})
	}
}

func TestWriteConfigFile_YAMLRejectsValuesAndMissingRaw(t *testing.T) {
	paths, _ := newConfigDirPaths(t)
	_, err := writeConfigFile(paths, "a.yml", domain.ConfigFileUpdate{
		Values: []domain.ConfigValueUpdate{{Section: "s", Key: "k", Value: "v"}},
	})
	var de *domain.Error
	if !errors.As(err, &de) || len(de.Fields) != 1 || de.Fields[0].Field != "values" {
		t.Fatalf("expected values field error, got %v", err)
	}
	if _, err := writeConfigFile(paths, "a.yml", domain.ConfigFileUpdate{}); err == nil {
		t.Error("expected error when neither raw nor values is given")
	}
}

func TestValidateConfigSyntax(t *testing.T) {
	cases := []struct {
		name    string
		format  domain.ConfigFileFormat
		raw     string
		wantErr string // substring; empty = valid
	}{
		{"yaml empty", domain.ConfigFormatYAML, "", ""},
		{"yaml comment only", domain.ConfigFormatYAML, "# hi\n", ""},
		{"yaml bad indent", domain.ConfigFormatYAML, "a:\n  b: 1\n c: 2\n", "invalid YAML"},
		{"yaml unclosed", domain.ConfigFormatYAML, "a: [1, 2\n", "line"},
		{"json ok", domain.ConfigFormatJSON, "{}", ""},
		{"json empty", domain.ConfigFormatJSON, "  \n", "invalid JSON: empty file"},
		{"json trailing comma", domain.ConfigFormatJSON, "{\n  \"a\": 1,\n}", "(line 3, column 2)"},
		{"json truncated", domain.ConfigFormatJSON, "{\"a\":", "invalid JSON: "},
	}
	for _, c := range cases {
		err := validateConfigSyntax(c.format, c.raw)
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.wantErr != "" && (err == nil || !strings.Contains(err.Error(), c.wantErr)):
			t.Errorf("%s: err=%v, want substring %q", c.name, err, c.wantErr)
		}
	}
}

func TestRemoveConfigFiles_YAML(t *testing.T) {
	paths, dir := newConfigDirPaths(t)
	for _, n := range []string{"X.yml", "Y.yml"} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("{}"), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	if err := removeConfigFiles(paths, []string{"X.yml", "missing.json"}); err != nil {
		t.Fatal(err)
	}
	if fileExists(filepath.Join(dir, "X.yml")) || !fileExists(filepath.Join(dir, "Y.yml")) {
		t.Error("only X.yml should have been removed")
	}
	if err := removeConfigFiles(paths, []string{"../evil.yml"}); err == nil {
		t.Error("expected validation error")
	}
}

func TestYAMLSyntaxError_PointsAtTheMistake(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "Azumatt.AzuAutoStore.yml"))
	if err != nil {
		t.Fatal(err)
	}
	good := string(fixture)
	lineOf := func(s, needle string) int {
		i := strings.Index(s, needle)
		if i < 0 {
			t.Fatalf("needle %q not found", needle)
		}
		return strings.Count(s[:i], "\n") + 1
	}
	cases := []struct {
		name     string
		old, new string
		wantLine func(broken string) int
	}{
		{
			name:     "wrong indent deep in the file",
			old:      "piece_chest_wood:\n  range: 15\n  exclude:",
			new:      "piece_chest_wood:\n  range: 15\n exclude:",
			wantLine: func(b string) int { return lineOf(b, "piece_chest_wood:") + 2 },
		},
		{
			name:     "unclosed flow sequence",
			old:      "piece_chest_private:\n  range: 10",
			new:      "piece_chest_private:\n  range: [10",
			wantLine: func(b string) int { return lineOf(b, "piece_chest_private:") + 1 },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(good, tc.old) {
				t.Fatalf("fixture no longer contains %q", tc.old)
			}
			broken := strings.Replace(good, tc.old, tc.new, 1)
			err := validateConfigSyntax(domain.ConfigFormatYAML, broken)
			if err == nil {
				t.Fatal("expected a syntax error")
			}
			want := fmt.Sprintf("near line %d:", tc.wantLine(broken))
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to contain %q", err, want)
			}
		})
	}
}

func TestYAMLSyntaxError_SkipsMultiLineConstructsAndBoundsWork(t *testing.T) {
	// A valid multi-line quoted scalar comes first: a prefix cut inside it
	// does not parse, but that is not the mistake.
	doc := "a: 1\nnote: \"first line\n  second line\n  third line\"\nb:\n  c: 2\n d: 3\ne: 4\n"
	err := validateConfigSyntax(domain.ConfigFormatYAML, doc)
	if err == nil {
		t.Fatal("expected a syntax error")
	}
	if want := "near line 7:"; !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q, want it to contain %q", err, want)
	}

	// Large inputs are rejected without the locating pass.
	big := strings.Repeat("k: v\n", (maxYAMLLocateBytes/5)+10) + " broken: [\n"
	err = validateConfigSyntax(domain.ConfigFormatYAML, big)
	if err == nil {
		t.Fatal("expected a syntax error for the large input")
	}
	if strings.Contains(err.Error(), "near line") {
		t.Errorf("large input should keep the parser's own message, got %q", err)
	}

	// The search stays logarithmic: count parses through a wrapper would
	// need a hook, so assert on time instead with a generous bound.
	mid := strings.Repeat("k: v\n", 20000) + " broken: [\n"
	if len(mid) > maxYAMLLocateBytes {
		t.Fatalf("test input (%d bytes) exceeds the locate limit", len(mid))
	}
	start := time.Now()
	if err := validateConfigSyntax(domain.ConfigFormatYAML, mid); err == nil || !strings.Contains(err.Error(), "near line 20001:") {
		t.Errorf("error = %v, want near line 20001", err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("locating took %s, want well under 5s", d)
	}
}
