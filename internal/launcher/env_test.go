package launcher

import (
	"os"
	"path/filepath"
	"testing"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read testdata %s: %v", name, err)
	}
	return b
}

func TestResolveBepInExEnv_5_4_2202(t *testing.T) {
	script := readTestdata(t, "start_server_bepinex_5.4.2202.sh")
	base := []string{"PATH=/usr/bin"}
	got, err := ResolveBepInExEnv(script, base)
	if err != nil {
		t.Fatalf("ResolveBepInExEnv: %v", err)
	}
	want := map[string]string{
		"PATH":                          "/usr/bin",
		"DOORSTOP_ENABLE":               "TRUE",
		"DOORSTOP_INVOKE_DLL_PATH":      "./BepInEx/core/BepInEx.Preloader.dll",
		"DOORSTOP_CORLIB_OVERRIDE_PATH": "./unstripped_corlib",
		"LD_LIBRARY_PATH":               "./doorstop_libs:",
		"LD_PRELOAD":                    "libdoorstop_x64.so:",
	}
	assertEnvEquals(t, got, want)
	// The lines after the second #### (including the exec line) must be
	// ignored entirely.
	if containsKey(got, "SteamAppId") {
		t.Errorf("SteamAppId leaked from after the marker block: %v", got)
	}
}

func TestResolveBepInExEnv_5_4_2333(t *testing.T) {
	script := readTestdata(t, "start_server_bepinex_5.4.2333.sh")
	base := []string{"LD_LIBRARY_PATH=./linux64:", "LD_PRELOAD=", "SteamAppId=892970"}
	got, err := ResolveBepInExEnv(script, base)
	if err != nil {
		t.Fatalf("ResolveBepInExEnv: %v", err)
	}
	want := map[string]string{
		"LD_LIBRARY_PATH":          "./doorstop_libs:./linux64:",
		"LD_PRELOAD":               "libdoorstop_x64.so:",
		"SteamAppId":               "892970",
		"DOORSTOP_ENABLED":         "1",
		"DOORSTOP_TARGET_ASSEMBLY": "./BepInEx/core/BepInEx.Preloader.dll",
	}
	assertEnvEquals(t, got, want)
}

func TestDefaultBepInExEnv(t *testing.T) {
	base := []string{"LD_LIBRARY_PATH=./linux64:"}
	got, err := DefaultBepInExEnv(base)
	if err != nil {
		t.Fatalf("DefaultBepInExEnv: %v", err)
	}
	want := map[string]string{
		"LD_LIBRARY_PATH":          "./doorstop_libs:./linux64:",
		"LD_PRELOAD":               "libdoorstop_x64.so:",
		"DOORSTOP_ENABLED":         "1",
		"DOORSTOP_TARGET_ASSEMBLY": "./BepInEx/core/BepInEx.Preloader.dll",
	}
	assertEnvEquals(t, got, want)
}

func TestResolveBepInExEnv_NoMarkers(t *testing.T) {
	_, err := ResolveBepInExEnv([]byte("echo hi\n"), nil)
	if err == nil {
		t.Fatal("expected an error for a script with no #### markers")
	}
}

func TestBaseEnv(t *testing.T) {
	got := baseEnv([]string{"FOO=bar", "LD_LIBRARY_PATH=/opt/lib"})
	want := map[string]string{
		"FOO":             "bar",
		"LD_LIBRARY_PATH": "./linux64:/opt/lib",
		"SteamAppId":      "892970",
	}
	assertEnvEquals(t, got, want)

	got2 := baseEnv(nil)
	want2 := map[string]string{
		"LD_LIBRARY_PATH": "./linux64:",
		"SteamAppId":      "892970",
	}
	assertEnvEquals(t, got2, want2)
}

func TestMaskedLaunchLine(t *testing.T) {
	line := maskedLaunchLine("./valheim_server.x86_64", []string{"-name", "Our Server", "-password", "s3cret", "-public", "1"})
	want := `[valheim-ui] launching valheim_server.x86_64 -name Our Server -password ******** -public 1`
	if line != want {
		t.Errorf("maskedLaunchLine =\n%q\nwant\n%q", line, want)
	}
}

func containsKey(env []string, key string) bool {
	_, m := envToOrdered(env)
	_, ok := m[key]
	return ok
}

func assertEnvEquals(t *testing.T, env []string, want map[string]string) {
	t.Helper()
	_, got := envToOrdered(env)
	for k, v := range want {
		if got[k] != v {
			t.Errorf("env[%s] = %q, want %q (full env: %v)", k, got[k], v, env)
		}
	}
}
