package mods

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

func TestExtractPackage_Rules(t *testing.T) {
	serverDir := t.TempDir()
	zipPath := filepath.Join(t.TempDir(), "corelib.zip")
	if err := os.WriteFile(zipPath, coreLibZip(t), 0o644); err != nil {
		t.Fatal(err)
	}

	written, err := extractPackage(zipPath, serverDir, "Alice", "CoreLib")
	if err != nil {
		t.Fatalf("extractPackage: %v", err)
	}
	sort.Strings(written)

	// plugins/CoreLib.dll must map to BepInEx/plugins/Alice-CoreLib/CoreLib.dll
	// (the "plugins/" prefix is replaced, not nested under an extra copy).
	assertHasFile(t, serverDir, "BepInEx/plugins/Alice-CoreLib/CoreLib.dll", "corelib-dll-v1")
	assertHasFile(t, serverDir, "BepInEx/config/corelib.cfg", "[General]\nEnabled = true\n")
	assertHasFile(t, serverDir, "BepInEx/plugins/Alice-CoreLib/manifest.json", "")
	assertHasFile(t, serverDir, "BepInEx/plugins/Alice-CoreLib/icon.png", "")
	assertHasFile(t, serverDir, "BepInEx/plugins/Alice-CoreLib/README.md", "")

	wantPaths := []string{
		"BepInEx/config/corelib.cfg",
		"BepInEx/plugins/Alice-CoreLib/CoreLib.dll",
		"BepInEx/plugins/Alice-CoreLib/README.md",
		"BepInEx/plugins/Alice-CoreLib/icon.png",
		"BepInEx/plugins/Alice-CoreLib/manifest.json",
	}
	sort.Strings(wantPaths)
	if !equalStrings(written, wantPaths) {
		t.Fatalf("unexpected written set:\n got  %v\n want %v", written, wantPaths)
	}
}

func TestExtractPackage_NeverOverwritesExistingConfig(t *testing.T) {
	serverDir := t.TempDir()
	cfgPath := filepath.Join(serverDir, "BepInEx", "config", "corelib.cfg")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte("user edited value\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	zipPath := filepath.Join(t.TempDir(), "corelib.zip")
	if err := os.WriteFile(zipPath, coreLibZip(t), 0o644); err != nil {
		t.Fatal(err)
	}
	written, err := extractPackage(zipPath, serverDir, "Alice", "CoreLib")
	if err != nil {
		t.Fatalf("extractPackage: %v", err)
	}
	for _, w := range written {
		if w == "BepInEx/config/corelib.cfg" {
			t.Error("existing cfg should not be recorded as (re)written")
		}
	}
	assertHasFile(t, serverDir, "BepInEx/config/corelib.cfg", "user edited value\n")
}

func TestExtractPackage_ZipSlipRejected(t *testing.T) {
	serverDir := t.TempDir()
	evil := buildZip(t, []zipEntry{{"../../etc/passwd", []byte("nope")}})
	zipPath := filepath.Join(t.TempDir(), "evil.zip")
	if err := os.WriteFile(zipPath, evil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := extractPackage(zipPath, serverDir, "Evil", "Zip"); err == nil {
		t.Fatal("expected a zip-slip error")
	}
	entries, _ := os.ReadDir(serverDir)
	if len(entries) != 0 {
		t.Errorf("zip-slip attempt should not have written anything, found %v", entries)
	}
}

func TestExtractBepInExPack_WrapperFolder(t *testing.T) {
	serverDir := t.TempDir()
	zipPath := filepath.Join(t.TempDir(), "bepinex.zip")
	if err := os.WriteFile(zipPath, bepinexPackZip(t), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := extractBepInExPack(zipPath, serverDir); err != nil {
		t.Fatalf("extractBepInExPack: %v", err)
	}
	assertHasFile(t, serverDir, "BepInEx/core/BepInEx.Preloader.dll", "preloader")
	assertHasFile(t, serverDir, "doorstop_libs/libdoorstop_x64.so", "doorstop")
	assertHasFile(t, serverDir, "start_server_bepinex.sh", "#!/bin/sh\n")
	assertHasFile(t, serverDir, "winhttp.dll", "winhttp")
	if !bepinexInstalled(domain.InstancePaths{Server: serverDir}) {
		t.Error("expected bepinexInstalled to detect the extracted loader")
	}
}

func TestSetFilesEnabled_TogglesDllSuffix(t *testing.T) {
	serverDir := t.TempDir()
	dllPath := filepath.Join(serverDir, "BepInEx", "plugins", "Alice-CoreLib", "CoreLib.dll")
	if err := os.MkdirAll(filepath.Dir(dllPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dllPath, []byte("dll"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []string{"BepInEx/plugins/Alice-CoreLib/CoreLib.dll", "BepInEx/plugins/Alice-CoreLib/README.md"}

	disabled, err := setFilesEnabled(serverDir, files, false)
	if err != nil {
		t.Fatalf("disable: %v", err)
	}
	if disabled[0] != "BepInEx/plugins/Alice-CoreLib/CoreLib.dll.disabled" {
		t.Errorf("expected .disabled suffix, got %q", disabled[0])
	}
	if !fileExists(filepath.Join(serverDir, filepath.FromSlash(disabled[0]))) {
		t.Error("expected renamed file to exist on disk")
	}
	if fileExists(dllPath) {
		t.Error("expected original dll path to no longer exist")
	}

	enabled, err := setFilesEnabled(serverDir, disabled, true)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if enabled[0] != "BepInEx/plugins/Alice-CoreLib/CoreLib.dll" {
		t.Errorf("expected suffix stripped back off, got %q", enabled[0])
	}
	if !fileExists(dllPath) {
		t.Error("expected dll restored to its original path")
	}
}

func TestRemoveManagedFiles_SkipsCfgAndPrunesEmptyDirs(t *testing.T) {
	serverDir := t.TempDir()
	files := []string{
		"BepInEx/plugins/Alice-CoreLib/CoreLib.dll",
		"BepInEx/plugins/Alice-CoreLib/README.md",
		"BepInEx/config/corelib.cfg",
	}
	for _, f := range files {
		abs := filepath.Join(serverDir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := removeManagedFiles(serverDir, files); err != nil {
		t.Fatalf("removeManagedFiles: %v", err)
	}

	if fileExists(filepath.Join(serverDir, "BepInEx/plugins/Alice-CoreLib")) {
		t.Error("expected the now-empty plugin dir to be pruned")
	}
	if !fileExists(filepath.Join(serverDir, "BepInEx/config/corelib.cfg")) {
		t.Error("cfg file must never be deleted by removeManagedFiles")
	}
}

func TestInstallSingleDLL(t *testing.T) {
	serverDir := t.TempDir()
	dllPath := filepath.Join(t.TempDir(), "MyMod.dll")
	if err := os.WriteFile(dllPath, []byte("dll-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	owner, name, files, err := installSingleDLL(dllPath, serverDir, "MyMod.dll")
	if err != nil {
		t.Fatalf("installSingleDLL: %v", err)
	}
	if owner != "local" || name != "mymod" {
		t.Errorf("unexpected identity: owner=%q name=%q", owner, name)
	}
	if len(files) != 1 || files[0] != "BepInEx/plugins/local-mymod/mymod.dll" {
		t.Errorf("unexpected files: %v", files)
	}
	assertHasFile(t, serverDir, "BepInEx/plugins/local-mymod/mymod.dll", "dll-bytes")
}

func TestManualZipIdentity(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "corelib.zip")
	if err := os.WriteFile(zipPath, coreLibZip(t), 0o644); err != nil {
		t.Fatal(err)
	}
	owner, name, version, deps, err := manualZipIdentity(zipPath)
	if err != nil {
		t.Fatalf("manualZipIdentity: %v", err)
	}
	if owner != "local" || name != "corelib" || version != "1.0.0" || len(deps) != 0 {
		t.Errorf("unexpected identity: %q %q %q %v", owner, name, version, deps)
	}
}

// --- helpers ---

func assertHasFile(t *testing.T, serverDir, rel, wantContent string) {
	t.Helper()
	abs := filepath.Join(serverDir, filepath.FromSlash(rel))
	data, err := os.ReadFile(abs) //nolint:gosec // test helper, path built from test-controlled fixtures
	if err != nil {
		t.Fatalf("expected file %s to exist: %v", rel, err)
	}
	if wantContent != "" && string(data) != wantContent {
		t.Errorf("file %s: got %q want %q", rel, string(data), wantContent)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
