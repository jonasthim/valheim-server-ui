package selfupdate

import (
	"strconv"
	"strings"
)

// parsedVersion is a decomposed "vX.Y.Z[-pre]" version string.
type parsedVersion struct {
	ok                  bool
	major, minor, patch int
	pre                 string // "" for a final release
}

// parseVersion parses "vX.Y.Z" or "X.Y.Z", with an optional "-pre" suffix
// (e.g. "v1.2.0-rc.1"). Anything else (including "dev") reports ok == false.
func parseVersion(v string) parsedVersion {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	core, pre, _ := strings.Cut(v, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return parsedVersion{}
	}
	nums := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return parsedVersion{}
		}
		nums[i] = n
	}
	return parsedVersion{ok: true, major: nums[0], minor: nums[1], patch: nums[2], pre: pre}
}

// Compare orders two version strings of the form "vX.Y.Z" or "vX.Y.Z-pre"
// (the "v" prefix is optional). It returns -1 if a < b, 0 if equal, 1 if
// a > b. A pre-release sorts before its final release (v1.2.0-rc.1 <
// v1.2.0). "dev" and any other string that does not parse as a version are
// treated as older than every parsable version (so a real release always
// looks newer than a dev build); two unparsable strings compare equal.
//
// Compare alone does not decide whether an upgrade may proceed from "dev" —
// Checker.CanSelfUpgrade refuses that regardless of what Compare reports.
func Compare(a, b string) int {
	pa, pb := parseVersion(a), parseVersion(b)
	switch {
	case !pa.ok && !pb.ok:
		return 0
	case !pa.ok:
		return -1
	case !pb.ok:
		return 1
	}
	if d := cmpInt(pa.major, pb.major); d != 0 {
		return d
	}
	if d := cmpInt(pa.minor, pb.minor); d != 0 {
		return d
	}
	if d := cmpInt(pa.patch, pb.patch); d != 0 {
		return d
	}
	switch {
	case pa.pre == "" && pb.pre == "":
		return 0
	case pa.pre == "": // a is a final release, b is a pre-release of the same X.Y.Z
		return 1
	case pb.pre == "":
		return -1
	default:
		return cmpInt(strings.Compare(pa.pre, pb.pre), 0)
	}
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
