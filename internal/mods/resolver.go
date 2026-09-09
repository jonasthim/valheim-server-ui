package mods

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// depPattern splits a Thunderstore dependency string "Owner-Name-Version"
// into its parts. Thunderstore owner (team) names never contain hyphens;
// package names occasionally do, so owner is anchored at the first segment
// and version at the trailing dotted-numeric segment.
var depPattern = regexp.MustCompile(`^([^-]+)-(.+)-(\d[\w.]*)$`)

// parseDependency splits "Owner-Name-Version" as used in a package version's
// dependencies array.
func parseDependency(dep string) (owner, name, version string, err error) {
	m := depPattern.FindStringSubmatch(dep)
	if m == nil {
		return "", "", "", fmt.Errorf("invalid dependency string %q", dep)
	}
	return m[1], m[2], m[3], nil
}

// compareVersions compares two dot-separated, mostly-numeric version
// strings part by part (ARCHITECTURE.md §12: "semver-ish compare by numeric
// dot parts"). Returns <0 if a<b, 0 if equal, >0 if a>b. Non-numeric parts
// compare as 0 (so "1.0.0-rc" and "1.0.0" compare equal on their numeric
// prefix); this is a deliberately simple ordering, not full semver.
func compareVersions(a, b string) int {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	n := len(pa)
	if len(pb) > n {
		n = len(pb)
	}
	for i := 0; i < n; i++ {
		var va, vb int
		if i < len(pa) {
			va = numericPrefix(pa[i])
		}
		if i < len(pb) {
			vb = numericPrefix(pb[i])
		}
		if va != vb {
			if va < vb {
				return -1
			}
			return 1
		}
	}
	return 0
}

func numericPrefix(s string) int {
	end := 0
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 0 {
		return 0
	}
	n, err := strconv.Atoi(s[:end])
	if err != nil {
		return 0
	}
	return n
}

// PlanStep is one package to install/upgrade as part of a dependency plan.
type PlanStep struct {
	Owner            string
	Name             string
	Version          string
	Dependencies     []string
	DownloadURL      string
	IconURL          string
	WebsiteURL       string
	AlreadyInstalled bool // satisfied by an existing mod row at >= Version; nothing to do
	Upgrade          bool // an existing mod row is present at a lower version
	FromVersion      string
}

// FullName is the Thunderstore identifier owner-name.
func (s PlanStep) FullName() string { return s.Owner + "-" + s.Name }

// resolvePlan performs a breadth-first walk of dependencies starting at
// owner/name@version (version == "" means latest), skipping the BepInEx pack
// itself (the caller installs it separately when missing) and skipping/
// upgrading already-installed mods per ARCHITECTURE.md §12. installed maps
// "owner-name" (lower-case) to the currently installed domain.Mod.
func resolvePlan(ctx context.Context, ts *Thunderstore, installed map[string]domain.Mod, owner, name, version string) ([]PlanStep, bool, error) {
	type queued struct{ owner, name, version string }

	visited := map[string]bool{}
	var plan []PlanStep
	needsBepInEx := false

	queue := []queued{{owner, name, version}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		key := strings.ToLower(cur.owner + "-" + cur.name)
		if key == strings.ToLower(domain.BepInExOwner+"-"+domain.BepInExName) {
			needsBepInEx = true
			continue
		}
		if visited[key] {
			continue
		}
		visited[key] = true

		pkg, err := ts.Package(ctx, cur.owner, cur.name)
		if err != nil {
			return nil, false, err
		}

		targetVersion := cur.version
		if targetVersion == "" {
			targetVersion = pkg.LatestVersion
		}
		var chosen *domain.PackageVersion
		for i := range pkg.Versions {
			if pkg.Versions[i].Version == targetVersion {
				chosen = &pkg.Versions[i]
				break
			}
		}
		if chosen == nil {
			return nil, false, domain.Ef(domain.CodePackageNotFound, "%s-%s version %s not found", cur.owner, cur.name, targetVersion)
		}

		step := PlanStep{
			Owner:        pkg.Owner,
			Name:         pkg.Name,
			Version:      chosen.Version,
			Dependencies: chosen.Dependencies,
			DownloadURL:  chosen.DownloadURL,
			IconURL:      pkg.IconURL,
			WebsiteURL:   pkg.WebsiteURL,
		}
		if ex, ok := installed[key]; ok {
			if compareVersions(ex.Version, chosen.Version) >= 0 {
				step.AlreadyInstalled = true
			} else {
				step.Upgrade = true
				step.FromVersion = ex.Version
			}
		}
		plan = append(plan, step)

		for _, dep := range chosen.Dependencies {
			dOwner, dName, dVersion, err := parseDependency(dep)
			if err != nil {
				return nil, false, fmt.Errorf("plan %s-%s@%s: %w", cur.owner, cur.name, chosen.Version, err)
			}
			queue = append(queue, queued{dOwner, dName, dVersion})
		}
	}
	return plan, needsBepInEx, nil
}
