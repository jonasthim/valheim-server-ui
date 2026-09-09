package steam

import (
	"fmt"
	"regexp"
	"strings"
)

// buildIDPattern matches a top-level "buildid" "<digits>" pair, used against
// the installed appmanifest_896660.acf.
var buildIDPattern = regexp.MustCompile(`"buildid"\s+"(\d+)"`)

// buildIDInline matches a "buildid" "<digits>" pair anywhere in a string,
// used once we have already narrowed the search to the public branch block.
var buildIDInline = regexp.MustCompile(`"buildid"\s*"(\d+)"`)

// fallbackPublicBuildID is used when the VDF can't be brace-matched (e.g. a
// truncated capture): it accepts the public branch's buildid as long as no
// closing brace appears first, per ARCHITECTURE.md §11.
var fallbackPublicBuildID = regexp.MustCompile(`"public"\s*\{[^}]*?"buildid"\s*"(\d+)"`)

// findBlock returns the contents between the matching { } that follow the
// first `"key"` occurrence in s, honouring nested braces. ok is false if key
// or its opening/closing brace can't be found.
func findBlock(s, key string) (block string, ok bool) {
	idx := strings.Index(s, `"`+key+`"`)
	if idx < 0 {
		return "", false
	}
	rest := s[idx+len(key)+2:]
	open := strings.IndexByte(rest, '{')
	if open < 0 {
		return "", false
	}
	depth := 0
	for i := open; i < len(rest); i++ {
		switch rest[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return rest[open+1 : i], true
			}
		}
	}
	return "", false
}

// parseLatestBuildID extracts the public branch's buildid from the output of
// `steamcmd +app_info_print <appid>`. It first walks branches -> public with
// brace matching (robust to other branches, such as a "beta" branch, sharing
// the "buildid" key) and falls back to a looser regex if the structure can't
// be brace-matched (e.g. unexpected/truncated output).
func parseLatestBuildID(output string) (string, error) {
	if branches, ok := findBlock(output, "branches"); ok {
		if public, ok := findBlock(branches, "public"); ok {
			if m := buildIDInline.FindStringSubmatch(public); m != nil {
				return m[1], nil
			}
		}
	}
	if m := fallbackPublicBuildID.FindStringSubmatch(output); m != nil {
		return m[1], nil
	}
	return "", fmt.Errorf("steam: could not find public branch buildid in app_info_print output")
}
