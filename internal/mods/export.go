package mods

import (
	"archive/zip"
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// r2xVersion is a package version split into the three numeric parts
// r2modman/Gale's export.r2x expects.
type r2xVersion struct {
	Major int `yaml:"major"`
	Minor int `yaml:"minor"`
	Patch int `yaml:"patch"`
}

// r2xMod is one mod entry of export.r2x.
type r2xMod struct {
	Name    string     `yaml:"name"`
	Version r2xVersion `yaml:"version"`
	Enabled bool       `yaml:"enabled"`
}

// r2xProfile is the r2modman/Gale profile export format (export.r2x).
type r2xProfile struct {
	ProfileName string   `yaml:"profileName"`
	Mods        []r2xMod `yaml:"mods"`
}

// semverPattern matches exactly three dot-separated numeric parts.
var semverPattern = regexp.MustCompile(`^(\d+)\.(\d+)\.(\d+)$`)

// parseVersion parses v as exactly three numeric dot-separated parts.
func parseVersion(v string) (major, minor, patch int, ok bool) {
	m := semverPattern.FindStringSubmatch(v)
	if m == nil {
		return 0, 0, 0, false
	}
	major, _ = strconv.Atoi(m[1])
	minor, _ = strconv.Atoi(m[2])
	patch, _ = strconv.Atoi(m[3])
	return major, minor, patch, true
}

// BuildR2Profile builds an r2modman/Gale-importable client profile zip:
// export.r2x (YAML) listing every installed Thunderstore mod at its exact
// version, plus one config/<name> entry per BepInEx config file. Mods not
// sourced from Thunderstore, or whose Version does not parse as exactly
// three numeric dot-separated parts, are omitted from the profile and
// returned in skipped (as Mod.FullName(), sorted, never nil) so the caller
// can tell the player what to install manually.
func BuildR2Profile(profileName string, mods []domain.Mod, configs []domain.ConfigFile) (zipBytes []byte, skipped []string, err error) {
	skipped = []string{}
	r2mods := make([]r2xMod, 0, len(mods))
	for _, m := range mods {
		if m.Source != domain.ModSourceThunderstore {
			skipped = append(skipped, m.FullName())
			continue
		}
		major, minor, patch, ok := parseVersion(m.Version)
		if !ok {
			skipped = append(skipped, m.FullName())
			continue
		}
		r2mods = append(r2mods, r2xMod{
			Name:    m.FullName(),
			Version: r2xVersion{Major: major, Minor: minor, Patch: patch},
			Enabled: m.Enabled,
		})
	}
	sort.Strings(skipped)

	yamlBytes, err := yaml.Marshal(r2xProfile{ProfileName: profileName, Mods: r2mods})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal export.r2x: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	r2xWriter, err := zw.Create("export.r2x")
	if err != nil {
		return nil, nil, fmt.Errorf("create export.r2x entry: %w", err)
	}
	if _, err := r2xWriter.Write(yamlBytes); err != nil {
		return nil, nil, fmt.Errorf("write export.r2x entry: %w", err)
	}
	for _, cf := range configs {
		cfgWriter, err := zw.Create("config/" + cf.Name)
		if err != nil {
			return nil, nil, fmt.Errorf("create config/%s entry: %w", cf.Name, err)
		}
		if _, err := cfgWriter.Write([]byte(cf.Raw)); err != nil {
			return nil, nil, fmt.Errorf("write config/%s entry: %w", cf.Name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, nil, fmt.Errorf("close r2 profile zip: %w", err)
	}
	return buf.Bytes(), skipped, nil
}
