package mods

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

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
	sortSectionsNaturally(entries)
	return entries
}

// sortSectionsNaturally reorders entries so sections appear in natural order
// ("1 - …", "2 - …", "10 - …") while every section keeps its keys in file
// order. BepInEx's ConfigFile.Save writes sections in plain string order,
// which puts "10" before "2" for mods that number their sections.
func sortSectionsNaturally(entries []domain.ConfigEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return naturalLess(entries[i].Section, entries[j].Section)
	})
}

// naturalLess compares two strings chunk by chunk, comparing runs of digits
// by numeric value and everything else case-insensitively.
func naturalLess(a, b string) bool {
	for a != "" && b != "" {
		ca, ra := naturalChunk(a)
		cb, rb := naturalChunk(b)
		a, b = ra, rb
		if ca == cb {
			continue
		}
		da, db := isDigits(ca), isDigits(cb)
		switch {
		case da && db:
			ta, tb := strings.TrimLeft(ca, "0"), strings.TrimLeft(cb, "0")
			if len(ta) != len(tb) {
				return len(ta) < len(tb)
			}
			if ta != tb {
				return ta < tb
			}
		case da != db:
			return da // digits sort before letters
		default:
			la, lb := strings.ToLower(ca), strings.ToLower(cb)
			if la != lb {
				return la < lb
			}
		}
	}
	return len(a) < len(b)
}

// naturalChunk splits off the leading run of digits or non-digits.
func naturalChunk(s string) (chunk, rest string) {
	digit := isDigits(s[:1])
	i := 1
	for i < len(s) && isDigits(s[i:i+1]) == digit {
		i++
	}
	return s[:i], s[i:]
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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

// configFormat maps a file name's (case-insensitive) extension to its format.
func configFormat(name string) (domain.ConfigFileFormat, bool) {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".cfg":
		return domain.ConfigFormatCfg, true
	case ".yml", ".yaml":
		return domain.ConfigFormatYAML, true
	case ".json":
		return domain.ConfigFormatJSON, true
	}
	return "", false
}

// configFileName validates a config file name from a URL path segment: a
// plain basename (no separators, not a dotfile) with a known config format.
func configFileName(name string) error {
	if name == "" || filepath.Base(name) != name || strings.HasPrefix(name, ".") {
		return domain.E(domain.CodeValidationFailed, "invalid config file name")
	}
	if _, ok := configFormat(name); !ok {
		return domain.E(domain.CodeValidationFailed, "invalid config file name")
	}
	return nil
}

// yamlLinePrefix matches the "line N: " yaml.v3 puts in front of a message.
var yamlLinePrefix = regexp.MustCompile(`^line (\d+): `)

// Locating a YAML mistake re-parses leading parts of the text, so the work
// is bounded: inputs above maxYAMLLocateBytes keep the parser's own position,
// and the search does a binary search plus a short look-ahead, at most
// yamlLocateRounds times (a few dozen parses in total, never one per line).
const (
	maxYAMLLocateBytes = 256 << 10
	yamlLocateAhead    = 20
	yamlLocateRounds   = 2
)

// yamlSyntaxError turns a yaml.v3 parse error into a message that points at
// the mistake. yaml.v3 reports the line where the enclosing block starts
// (its context mark), which for a wrong indent deep in a file is far above
// the offending line. The real position is the line after the longest
// leading run of whole lines that still parses. That boundary is found by
// binary search (the empty text parses, the whole text does not). A cut in
// the middle of a multi-line construct (quoted string, flow collection) also
// fails, so after each search a few following prefixes are tried: if one
// parses, the boundary was such a cut and the search continues after it.
func yamlSyntaxError(raw string, err error) error {
	return yamlSyntaxErrorWithParser(raw, err, yamlParses)
}

func yamlSyntaxErrorWithParser(raw string, err error, parse func(string) bool) error {
	msg := strings.TrimPrefix(err.Error(), "yaml: ")
	if len(raw) > maxYAMLLocateBytes {
		return fmt.Errorf("invalid YAML: %s", msg)
	}
	reason := yamlLinePrefix.ReplaceAllString(msg, "")

	// ends[n] is the byte offset just after line n (ends[0] = 0), so
	// raw[:ends[n]] is the first n lines without any copying.
	ends := []int{0}
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\n' {
			ends = append(ends, i+1)
		}
	}
	if ends[len(ends)-1] != len(raw) {
		ends = append(ends, len(raw))
	}
	total := len(ends) - 1
	parses := func(n int) bool { return parse(raw[:ends[n]]) }

	lo, hi := 0, total // invariant: the first lo lines parse, the first hi do not
	for round := 0; ; round++ {
		for hi-lo > 1 {
			mid := lo + (hi-lo)/2
			if parses(mid) {
				lo = mid
			} else {
				hi = mid
			}
		}
		if round >= yamlLocateRounds {
			break
		}
		resumed := false
		for m := lo + 2; m < total && m <= lo+1+yamlLocateAhead; m++ {
			if parses(m) {
				lo, hi, resumed = m, total, true
				break
			}
		}
		if !resumed {
			break
		}
	}
	return fmt.Errorf("invalid YAML near line %d: %s", lo+1, reason)
}

// yamlParses reports whether every document in raw parses.
func yamlParses(raw string) bool {
	dec := yaml.NewDecoder(strings.NewReader(raw))
	for {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			return true
		}
		if err != nil {
			return false
		}
	}
}

// validateConfigSyntax checks that raw is syntactically valid for a yaml or
// json config file. The text itself is never rewritten.
func validateConfigSyntax(format domain.ConfigFileFormat, raw string) error {
	switch format {
	case domain.ConfigFormatYAML:
		dec := yaml.NewDecoder(strings.NewReader(raw))
		for {
			var n yaml.Node
			err := dec.Decode(&n)
			if errors.Is(err, io.EOF) {
				return nil
			}
			if err != nil {
				return yamlSyntaxError(raw, err)
			}
		}
	case domain.ConfigFormatJSON:
		if strings.TrimSpace(raw) == "" {
			return errors.New("invalid JSON: empty file")
		}
		var v any
		err := json.Unmarshal([]byte(raw), &v)
		if err == nil {
			return nil
		}
		var se *json.SyntaxError
		if errors.As(err, &se) {
			line, col := 1, 1
			end := min(int(se.Offset), len(raw))
			for i := 0; i < end; i++ {
				if raw[i] == '\n' {
					line++
					col = 1
				} else {
					col++
				}
			}
			return fmt.Errorf("invalid JSON: %v (line %d, column %d)", err, line, col)
		}
		return fmt.Errorf("invalid JSON: %v", err)
	}
	return nil
}

// removeConfigFiles deletes the named config files from the instance's config
// dir. Each name is validated as a plain config basename (.cfg/.yml/.yaml/.json); a missing file is not
// an error (the mod may never have written it).
func removeConfigFiles(paths domain.InstancePaths, names []string) error {
	for _, name := range names {
		if err := configFileName(name); err != nil {
			return err
		}
		p := filepath.Join(configDir(paths), name)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove config %s: %w", name, err)
		}
	}
	return nil
}

func configDir(paths domain.InstancePaths) string {
	return filepath.Join(paths.BepInExDir(), "config")
}

// listConfigFiles lists the regular config files (*.cfg, *.yml, *.yaml, *.json)
// directly in server/BepInEx/config; subdirectories, symlinks and dotfiles are skipped.
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
		if !e.Type().IsRegular() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		format, ok := configFormat(e.Name())
		if !ok {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, domain.ConfigFileInfo{
			Name:       e.Name(),
			Format:     format,
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
	data, err := os.ReadFile(p) //nolint:gosec // name validated by configFileName (plain basename, known config extension)
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
	format, _ := configFormat(name) // known: validated by configFileName
	entries := []domain.ConfigEntry{}
	if format == domain.ConfigFormatCfg {
		entries = parseCfg(raw)
	}
	return &domain.ConfigFile{
		Name:       name,
		Format:     format,
		Raw:        raw,
		Entries:    entries,
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
	format, _ := configFormat(name) // known: validated by configFileName

	if format != domain.ConfigFormatCfg {
		if len(upd.Values) > 0 {
			return nil, domain.Validation([]domain.FieldError{{Field: "values", Message: "only cfg files support key updates; send raw"}})
		}
		if upd.Raw == nil {
			return nil, domain.E(domain.CodeValidationFailed, "either raw or values must be provided")
		}
		if err := validateConfigSyntax(format, *upd.Raw); err != nil {
			return nil, domain.Validation([]domain.FieldError{{Field: "raw", Message: err.Error()}})
		}
	}

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
