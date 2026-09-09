package players

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// idPattern is the allowed shape of a platform id in a list file.
var idPattern = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// parsedListFile is the file-level view used to preserve comments across a
// write: entries for the API, plus enough of the original layout to
// reproduce untouched comments verbatim.
type parsedListFile struct {
	// header holds standalone "//" comment lines that appear before the
	// first entry (e.g. a file banner). Printed back verbatim, once, before
	// any entries.
	header []string
	// entries are the ids in file order; Comment is populated only from a
	// trailing "// ..." on the same line (the API's documented meaning).
	entries []domain.PlayerListEntry
	// preceding maps an id to a standalone comment line that appeared
	// immediately above it (and not as its trailing comment). Re-emitted
	// verbatim on write for entries that keep the same id and get no new
	// explicit Comment from the caller.
	preceding map[string]string
}

// parseListFile reads and parses path. A missing file is not an error: it
// parses as empty (ReadList then returns an empty list; WriteList creates
// the file).
func parseListFile(path string) (parsedListFile, error) {
	pf := parsedListFile{preceding: map[string]string{}}

	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return pf, nil
	}
	if err != nil {
		return pf, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1<<20)

	var pendingComment string
	haveEntry := false
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//") {
			if !haveEntry {
				pf.header = append(pf.header, line)
			} else {
				pendingComment = line
			}
			continue
		}

		id := line
		comment := ""
		if idx := strings.Index(line, "//"); idx >= 0 {
			id = strings.TrimSpace(line[:idx])
			comment = strings.TrimSpace(line[idx+2:])
		}
		if !idPattern.MatchString(id) {
			return parsedListFile{}, fmt.Errorf("%s: invalid player id %q", path, id)
		}
		if comment == "" && pendingComment != "" {
			pf.preceding[id] = pendingComment
		}
		pendingComment = ""
		pf.entries = append(pf.entries, domain.PlayerListEntry{ID: id, Comment: comment})
		haveEntry = true
	}
	if err := sc.Err(); err != nil {
		return parsedListFile{}, fmt.Errorf("read %s: %w", path, err)
	}
	return pf, nil
}

// ReadList returns the current contents of the list file for kind under
// paths.Save. A missing file reads as an empty list, not an error.
func ReadList(path string, kind domain.ListKind) (*domain.PlayerList, error) {
	pf, err := parseListFile(path)
	if err != nil {
		return nil, domain.Wrap(domain.CodeInternal, "read player list", err)
	}
	entries := pf.entries
	if entries == nil {
		entries = []domain.PlayerListEntry{}
	}
	return &domain.PlayerList{Kind: kind, Entries: entries}, nil
}

// validateEntries checks every id against idPattern and rejects duplicates,
// collecting all problems into one validation error.
func validateEntries(entries []domain.PlayerListEntry) error {
	var fields []domain.FieldError
	seen := map[string]bool{}
	for i, e := range entries {
		if !idPattern.MatchString(e.ID) {
			fields = append(fields, domain.FieldError{
				Field:   fmt.Sprintf("entries[%d].id", i),
				Message: "must match ^[A-Za-z0-9_]{1,64}$",
			})
			continue
		}
		if seen[e.ID] {
			fields = append(fields, domain.FieldError{
				Field:   fmt.Sprintf("entries[%d].id", i),
				Message: "duplicate id",
			})
		}
		seen[e.ID] = true
	}
	return domain.Validation(fields)
}

// WriteList validates list and atomically replaces the file at path,
// preserving a leading comment header verbatim and, for any entry that keeps
// the same id and specifies no new Comment, whatever standalone comment line
// preceded it before.
func WriteList(path string, list domain.PlayerList) error {
	if err := validateEntries(list.Entries); err != nil {
		return err
	}

	old, err := parseListFile(path)
	if err != nil {
		// A hand-edited file that no longer parses must not block writing a
		// new, valid one; comment preservation is simply lost in that case.
		old = parsedListFile{preceding: map[string]string{}}
	}

	var b strings.Builder
	for _, h := range old.header {
		b.WriteString(h)
		b.WriteByte('\n')
	}
	if len(old.header) > 0 {
		b.WriteByte('\n')
	}
	for _, e := range list.Entries {
		switch {
		case e.Comment != "":
			fmt.Fprintf(&b, "%s // %s\n", e.ID, e.Comment)
		case old.preceding[e.ID] != "":
			b.WriteString(old.preceding[e.ID])
			b.WriteByte('\n')
			b.WriteString(e.ID)
			b.WriteByte('\n')
		default:
			b.WriteString(e.ID)
			b.WriteByte('\n')
		}
	}

	return atomicWriteFile(path, []byte(b.String()), 0o640)
}

// atomicWriteFile writes data to a temp file in the same directory as path
// and renames it into place, creating path (and its directory) if missing.
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	cleanTemp := true
	defer func() {
		if cleanTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("rename into place: %w", err)
	}
	cleanTemp = false
	return nil
}
