package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Valheim stores a world in one of two layouts under save/worlds_local:
//
//   - legacy (before l-1.0.7): a flat <World>.fwl + <World>.db pair, plus
//     .old siblings and <World>_backup_auto-<ts>.* rolling copies;
//   - directory (l-1.0.7+): a <World>/ directory holding one committed save
//     generation _main.<N>.fwl2 / .db2 / .chunks / .ok plus many
//     <x>_<y>__<kind>_<gen>.chunk files, where each chunk carries its own
//     generation and is only rewritten when dirty. The game writes
//     _main.<N>.ok last, so its presence is the commit marker. Rolling
//     copies are sibling directories <World>_backup_auto-<ts>/.
//
// A migrated pre-1.0 world keeps its legacy files next to the new directory;
// the directory is then authoritative. This file is the only place that
// knows either layout; everything else asks worldSave.

// worldSave is what scanWorlds found for one world name.
type worldSave struct {
	Name string

	// Legacy flat pair.
	LegacyDB  bool
	LegacyFWL bool

	// Directory layout. Generation is the highest committed generation
	// (its _main.<N>.ok exists), 0 when none is committed yet.
	Dir        bool
	Generation int
	DirDB      bool // _main.<Generation>.db2 exists
	DirFWL     bool // _main.<Generation>.fwl2 exists

	SizeBytes  int64
	ModifiedAt time.Time
}

// HasDB reports whether the world has save data a backup can capture.
func (w worldSave) HasDB() bool { return w.DirDB || w.LegacyDB }

// HasFWL reports whether the world has its metadata file.
func (w worldSave) HasFWL() bool { return w.DirFWL || w.LegacyFWL }

// exists reports whether anything at all belongs to this world on disk.
func (w worldSave) exists() bool { return w.Dir || w.LegacyDB || w.LegacyFWL }

// mainFilePattern matches _main.<N>.<ext> inside a world directory.
var mainFilePattern = regexp.MustCompile(`^_main\.(\d+)\.(fwl2|db2|chunks|ok)$`)

// scanWorlds reads worlds_local (dir) once and groups everything it finds by
// world name. A world is listed when it has at least one legacy file or at
// least one _main.* file in its directory; an empty directory is not a world.
// Valheim's own rolling copies are skipped in both layouts. A missing dir
// yields an empty map.
func scanWorlds(dir string) (map[string]*worldSave, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]*worldSave{}, nil
		}
		return nil, fmt.Errorf("read worlds directory: %w", err)
	}

	out := map[string]*worldSave{}
	get := func(name string) *worldSave {
		w := out[name]
		if w == nil {
			w = &worldSave{Name: name}
			out[name] = w
		}
		return w
	}
	touch := func(w *worldSave, fi os.FileInfo) {
		w.SizeBytes += fi.Size()
		if fi.ModTime().After(w.ModifiedAt) {
			w.ModifiedAt = fi.ModTime()
		}
	}

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() {
			if isValheimBackupStem(name) {
				continue
			}
			w, err := scanWorldDir(filepath.Join(dir, name), name)
			if err != nil {
				return nil, err
			}
			if w == nil {
				continue
			}
			existing := out[name]
			if existing == nil {
				out[name] = w
				continue
			}
			existing.Dir, existing.Generation, existing.DirDB, existing.DirFWL = true, w.Generation, w.DirDB, w.DirFWL
			existing.SizeBytes += w.SizeBytes
			if w.ModifiedAt.After(existing.ModifiedAt) {
				existing.ModifiedAt = w.ModifiedAt
			}
			continue
		}

		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".db" && ext != ".fwl" {
			continue // .old siblings deliberately excluded from the world list
		}
		stem := strings.TrimSuffix(name, filepath.Ext(name))
		if isValheimBackupStem(stem) {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		w := get(stem)
		if ext == ".db" {
			w.LegacyDB = true
		} else {
			w.LegacyFWL = true
		}
		touch(w, fi)
	}
	return out, nil
}

// scanWorldDir inspects one <World>/ directory. It returns nil when the
// directory holds no _main.* file at all.
func scanWorldDir(path, name string) (*worldSave, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("read world directory %s: %w", name, err)
	}
	w := &worldSave{Name: name, Dir: true}
	type gen struct{ ok, db, fwl bool }
	gens := map[int]*gen{}
	sawMain := false
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		w.SizeBytes += fi.Size()
		if fi.ModTime().After(w.ModifiedAt) {
			w.ModifiedAt = fi.ModTime()
		}
		m := mainFilePattern.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		sawMain = true
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		g := gens[n]
		if g == nil {
			g = &gen{}
			gens[n] = g
		}
		switch m[2] {
		case "ok":
			g.ok = true
		case "db2":
			g.db = true
		case "fwl2":
			g.fwl = true
		}
	}
	if !sawMain {
		return nil, nil
	}
	for n, g := range gens {
		if g.ok && n > w.Generation {
			w.Generation, w.DirDB, w.DirFWL = n, g.db, g.fwl
		}
	}
	return w, nil
}

// scanWorld is scanWorlds narrowed to one name; it returns a zero worldSave
// (exists() == false) when nothing belongs to that world.
func scanWorld(dir, world string) (worldSave, error) {
	all, err := scanWorlds(dir)
	if err != nil {
		return worldSave{}, err
	}
	if w := all[world]; w != nil {
		return *w, nil
	}
	return worldSave{Name: world}, nil
}

// toDomain renders the listing entry.
func (w worldSave) toDomain(active bool) domain.World {
	return domain.World{
		Name:       w.Name,
		Active:     active,
		SizeBytes:  w.SizeBytes,
		ModifiedAt: w.ModifiedAt.UTC(),
		HasDB:      w.HasDB(),
		HasFWL:     w.HasFWL(),
	}
}
