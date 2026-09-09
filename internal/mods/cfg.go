package mods

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

var (
	sectionPattern    = regexp.MustCompile(`^\s*\[(.+)\]\s*$`)
	settingTypePrefix = "# Setting type:"
	defaultValPrefix  = "# Default value:"
	acceptValsPrefix  = "# Acceptable values:"
	acceptRangePrefix = "# Acceptable value range:"
	rangePattern      = regexp.MustCompile(`(?i)from\s+(\S+)\s+to\s+(\S+)`)
	// keyValuePattern captures (indent)(key)(eqAndSpacing)(value) so an
	// update can rewrite only the value, byte-for-byte preserving everything
	// else on the line (ARCHITECTURE.md §12).
	keyValuePattern = regexp.MustCompile(`^(\s*)([^=\s#\[][^=]*?)(\s*=\s*)(.*)$`)
)

// cfgLine is one physical line of a .cfg file plus the exact terminator it
// had in the source (needed to reproduce the file byte-for-byte on update).
type cfgLine struct {
	text string // without terminator
	term string // "\n", "\r\n", or "" for a final line with no trailing newline
}

func splitLines(raw string) []cfgLine {
	var lines []cfgLine
	start := 0
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\n' {
			continue
		}
		end := i
		term := "\n"
		if end > start && raw[end-1] == '\r' {
			end--
			term = "\r\n"
		}
		lines = append(lines, cfgLine{text: raw[start:end], term: term})
		start = i + 1
	}
	if start < len(raw) {
		lines = append(lines, cfgLine{text: raw[start:], term: ""})
	}
	return lines
}

func joinLines(lines []cfgLine) string {
	var b strings.Builder
	for _, l := range lines {
		b.WriteString(l.text)
		b.WriteString(l.term)
	}
	return b.String()
}

// parseCfg parses a BepInEx .cfg file's content into entries (ARCHITECTURE.md
// §12): "[Section]" headers, "## description" lines (joined with spaces when
// consecutive), "# Setting type:", "# Default value:", "# Acceptable
// values:", "# Acceptable value range: From X to Y" metadata comments
// immediately preceding a "Key = Value" line.
func parseCfg(raw string) []domain.ConfigEntry {
	var entries []domain.ConfigEntry
	var section string
	var descLines []string
	var settingType, defaultValue string
	var acceptable []string
	var rng *domain.ConfigRange

	resetPending := func() {
		descLines = nil
		settingType = ""
		defaultValue = ""
		acceptable = nil
		rng = nil
	}

	for _, l := range splitLines(raw) {
		line := l.text
		trimmed := strings.TrimSpace(line)

		if m := sectionPattern.FindStringSubmatch(line); m != nil {
			section = strings.TrimSpace(m[1])
			resetPending()
			continue
		}
		if strings.HasPrefix(trimmed, "##") {
			descLines = append(descLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "##")))
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			switch {
			case strings.HasPrefix(trimmed, settingTypePrefix):
				settingType = strings.TrimSpace(strings.TrimPrefix(trimmed, settingTypePrefix))
			case strings.HasPrefix(trimmed, defaultValPrefix):
				defaultValue = strings.TrimSpace(strings.TrimPrefix(trimmed, defaultValPrefix))
			case strings.HasPrefix(trimmed, acceptRangePrefix):
				if m := rangePattern.FindStringSubmatch(trimmed); m != nil {
					rng = &domain.ConfigRange{Min: m[1], Max: m[2]}
				}
			case strings.HasPrefix(trimmed, acceptValsPrefix):
				rest := strings.TrimSpace(strings.TrimPrefix(trimmed, acceptValsPrefix))
				acceptable = nil
				for _, v := range strings.Split(rest, ",") {
					v = strings.TrimSpace(v)
					if v != "" {
						acceptable = append(acceptable, v)
					}
				}
			}
			continue
		}
		if trimmed == "" {
			continue
		}
		if m := keyValuePattern.FindStringSubmatch(line); m != nil {
			key := strings.TrimSpace(m[2])
			value := m[4]
			entries = append(entries, domain.ConfigEntry{
				Section:          section,
				Key:              key,
				Value:            value,
				Description:      strings.Join(descLines, " "),
				Type:             settingType,
				DefaultValue:     defaultValue,
				AcceptableValues: acceptable,
				Range:            rng,
			})
			resetPending()
		}
	}
	return entries
}

// applyCfgValues rewrites raw, replacing only the value of each matching
// "Key = Value" line inside the right section, byte-for-byte preserving
// every other line (comments, blank lines, unrelated keys, original line
// terminators). Returns a validation error listing any (section, key) that
// does not exist in raw.
func applyCfgValues(raw string, updates []domain.ConfigValueUpdate) (string, error) {
	known := map[string]bool{}
	for _, e := range parseCfg(raw) {
		known[e.Section+"\x00"+e.Key] = true
	}
	var fields []domain.FieldError
	pending := map[string]string{} // "section\x00key" -> new value, first match wins
	for i, u := range updates {
		k := u.Section + "\x00" + u.Key
		if !known[k] {
			fields = append(fields, domain.FieldError{
				Field:   fmt.Sprintf("values[%d]", i),
				Message: fmt.Sprintf("unknown key %q in section %q", u.Key, u.Section),
			})
			continue
		}
		pending[k] = u.Value
	}
	if len(fields) > 0 {
		return "", domain.Validation(fields)
	}

	lines := splitLines(raw)
	var section string
	for i, l := range lines {
		if m := sectionPattern.FindStringSubmatch(l.text); m != nil {
			section = strings.TrimSpace(m[1])
			continue
		}
		trimmed := strings.TrimSpace(l.text)
		if strings.HasPrefix(trimmed, "#") || trimmed == "" {
			continue
		}
		m := keyValuePattern.FindStringSubmatch(l.text)
		if m == nil {
			continue
		}
		key := strings.TrimSpace(m[2])
		newValue, ok := pending[section+"\x00"+key]
		if !ok {
			continue
		}
		lines[i].text = m[1] + m[2] + m[3] + newValue
	}
	return joinLines(lines), nil
}

// configFileName validates a config file name from a URL path segment: a
// plain basename ending in .cfg, no path separators.
func configFileName(name string) error {
	if name == "" || filepath.Base(name) != name || !strings.HasSuffix(name, ".cfg") {
		return domain.E(domain.CodeValidationFailed, "invalid config file name")
	}
	return nil
}

func configDir(paths domain.InstancePaths) string {
	return filepath.Join(paths.BepInExDir(), "config")
}

// listConfigFiles lists server/BepInEx/config/*.cfg.
func listConfigFiles(paths domain.InstancePaths) ([]domain.ConfigFileInfo, error) {
	dir := configDir(paths)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []domain.ConfigFileInfo{}, nil
		}
		return nil, fmt.Errorf("list config dir: %w", err)
	}
	out := make([]domain.ConfigFileInfo, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".cfg") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, domain.ConfigFileInfo{
			Name:       e.Name(),
			SizeBytes:  info.Size(),
			ModifiedAt: info.ModTime().UTC(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func readConfigFile(paths domain.InstancePaths, name string) (*domain.ConfigFile, error) {
	if err := configFileName(name); err != nil {
		return nil, err
	}
	p := filepath.Join(configDir(paths), name)
	data, err := os.ReadFile(p) //nolint:gosec // name validated by configFileName (plain basename, .cfg suffix)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, domain.NotFound("config file")
		}
		return nil, fmt.Errorf("read config file: %w", err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		return nil, fmt.Errorf("stat config file: %w", err)
	}
	raw := string(data)
	return &domain.ConfigFile{
		Name:       name,
		Raw:        raw,
		Entries:    parseCfg(raw),
		ModifiedAt: fi.ModTime().UTC(),
	}, nil
}

// writeConfigFile applies upd to name's content (raw replaces wholesale;
// values rewrites in place) and returns the resulting ConfigFile.
func writeConfigFile(paths domain.InstancePaths, name string, upd domain.ConfigFileUpdate) (*domain.ConfigFile, error) {
	if err := configFileName(name); err != nil {
		return nil, err
	}
	p := filepath.Join(configDir(paths), name)

	var newRaw string
	switch {
	case upd.Raw != nil:
		newRaw = *upd.Raw
	case len(upd.Values) > 0:
		cur, err := readConfigFile(paths, name)
		if err != nil {
			return nil, err
		}
		newRaw, err = applyCfgValues(cur.Raw, upd.Values)
		if err != nil {
			return nil, err
		}
	default:
		return nil, domain.E(domain.CodeValidationFailed, "either raw or values must be provided")
	}

	if err := writeFileAtomic(p, []byte(newRaw), 0o640); err != nil {
		return nil, fmt.Errorf("write config file: %w", err)
	}
	return readConfigFile(paths, name)
}
