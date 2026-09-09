package launcher

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// exportLineRE matches `export KEY=VALUE` lines inside a doorstop launcher
// script. Leading whitespace is tolerated; the value runs to end of line.
var exportLineRE = regexp.MustCompile(`^export\s+([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)

// defaultBepInExScript are the doorstop environment variables to apply when
// `bepinex` is set in launch.json but `start_server_bepinex.sh` is missing
// from the server directory. These match BepInEx 5.4.23xx (ARCHITECTURE.md
// §7). It is expressed as a script fragment so it can be parsed by the exact
// same code path as a real launcher script.
const defaultBepInExScript = `#!/bin/bash
####
export DOORSTOP_ENABLED=1
export DOORSTOP_TARGET_ASSEMBLY=./BepInEx/core/BepInEx.Preloader.dll
export LD_LIBRARY_PATH="./doorstop_libs:$LD_LIBRARY_PATH"
export LD_PRELOAD="libdoorstop_x64.so:$LD_PRELOAD"
####
`

// extractExports returns the (key, rawValue) pairs of every `export KEY=VALUE`
// line found strictly between the first and second lines that are exactly
// "####" (after trimming surrounding whitespace). rawValue is unprocessed:
// still possibly double-quoted and containing $VAR/${VAR} references.
func extractExports(script []byte) ([][2]string, error) {
	lines := strings.Split(string(script), "\n")
	var markers []int
	for i, l := range lines {
		if strings.TrimSpace(l) == "####" {
			markers = append(markers, i)
			if len(markers) == 2 {
				break
			}
		}
	}
	if len(markers) < 2 {
		return nil, fmt.Errorf("launcher: no #### marker pair found in bepinex script")
	}
	var out [][2]string
	for _, l := range lines[markers[0]+1 : markers[1]] {
		m := exportLineRE.FindStringSubmatch(strings.TrimSpace(l))
		if m == nil {
			continue
		}
		out = append(out, [2]string{m[1], m[2]})
	}
	return out, nil
}

// unquote strips one pair of surrounding double quotes, if present.
func unquote(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}

// envToOrdered splits "KEY=VALUE" env entries into an ordered key list (first
// occurrence order) and a map of the latest value per key.
func envToOrdered(env []string) ([]string, map[string]string) {
	order := make([]string, 0, len(env))
	m := make(map[string]string, len(env))
	for _, kv := range env {
		key, val, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, exists := m[key]; !exists {
			order = append(order, key)
		}
		m[key] = val
	}
	return order, m
}

// orderedToEnv renders order/m back into "KEY=VALUE" entries.
func orderedToEnv(order []string, m map[string]string) []string {
	out := make([]string, 0, len(order))
	for _, k := range order {
		out = append(out, k+"="+m[k])
	}
	return out
}

// ResolveBepInExEnv is the pure core of the launcher's doorstop handling
// (ARCHITECTURE.md §7 step 2). It parses every `export KEY=VALUE` line
// between the script's first two "####" markers, expands $VAR/${VAR}
// references against baseEnv plus exports already applied earlier in the
// same script (so later lines can see earlier ones), unquotes double-quoted
// values, and returns baseEnv with those keys added or overridden. Order of
// pre-existing keys is preserved; newly introduced keys are appended in the
// order they were exported.
func ResolveBepInExEnv(script []byte, baseEnv []string) ([]string, error) {
	exports, err := extractExports(script)
	if err != nil {
		return nil, err
	}
	order, m := envToOrdered(baseEnv)
	for _, kv := range exports {
		key, raw := kv[0], kv[1]
		val := unquote(raw)
		val = os.Expand(val, func(name string) string { return m[name] })
		if _, exists := m[key]; !exists {
			order = append(order, key)
		}
		m[key] = val
	}
	return orderedToEnv(order, m), nil
}

// DefaultBepInExEnv applies the built-in BepInEx 5.4.23xx defaults (used when
// the server directory has no start_server_bepinex.sh) on top of baseEnv.
func DefaultBepInExEnv(baseEnv []string) ([]string, error) {
	return ResolveBepInExEnv([]byte(defaultBepInExScript), baseEnv)
}
