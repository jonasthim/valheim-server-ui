package mapstyle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
)

// Pack is an optional folder of PNG textures that replace the builtin ones
// role by role. A missing or unreadable file leaves that role builtin; the
// problem is recorded so the caller can log it once.
type Pack struct {
	Dir         string
	tiles       map[Role]*Tile
	fingerprint string
	Problems    []string
}

// packConfig is the optional pack.json.
type packConfig struct {
	MetresPerTile map[string]float32 `json:"metres_per_tile"`
}

const packConfigName = "pack.json"

func roleFile(r Role) string { return string(r) + ".png" }

// maxPackTexels bounds a pack texture (2048²).
const maxPackTexels = 2048 * 2048

// LoadPack reads dir. A missing directory yields an empty pack ("builtin").
func LoadPack(dir string) (*Pack, error) {
	p := &Pack{Dir: dir, tiles: map[Role]*Tile{}}
	fp, err := Fingerprint(dir)
	if err != nil {
		return nil, err
	}
	p.fingerprint = fp
	if fp == "builtin" {
		return p, nil
	}
	cfg := packConfig{}
	if raw, err := os.ReadFile(filepath.Join(dir, packConfigName)); err == nil { //nolint:gosec // admin-controlled folder inside the instance
		if err := json.Unmarshal(raw, &cfg); err != nil {
			p.Problems = append(p.Problems, packConfigName+": "+err.Error())
		}
	}
	for _, role := range AllRoles {
		path := filepath.Join(dir, roleFile(role))
		f, err := os.Open(path) //nolint:gosec // admin-controlled folder inside the instance
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				p.Problems = append(p.Problems, roleFile(role)+": "+err.Error())
			}
			continue
		}
		cfgImg, err := png.DecodeConfig(f)
		if err == nil && cfgImg.Width*cfgImg.Height > maxPackTexels {
			err = fmt.Errorf("larger than 2048x2048")
		}
		var t *Tile
		if err == nil {
			if _, serr := f.Seek(0, 0); serr == nil {
				img, derr := png.Decode(f)
				if derr != nil {
					err = derr
				} else {
					metres := defaultMetres(role)
					if v, ok := cfg.MetresPerTile[string(role)]; ok && v > 0 {
						metres = v
					}
					t = tileFromImage(img, metres)
				}
			}
		}
		_ = f.Close()
		if err != nil {
			p.Problems = append(p.Problems, roleFile(role)+": "+err.Error())
			continue
		}
		if role == RoleForestTree {
			treePriority(t)
		}
		p.tiles[role] = t
	}
	return p, nil
}

// treePriority gives a pack's tree texture the per-disc priority the builtin
// carries in Aux: a hash of the 16-texel block, so density thresholds still
// thin the forest coherently.
func treePriority(t *Tile) {
	t.Aux = make([]float32, t.W*t.H)
	n := newNoise(0x7ee5)
	for y := 0; y < t.H; y++ {
		for x := 0; x < t.W; x++ {
			t.Aux[y*t.W+x] = n.at(x/16*3+1, y/16*3+2)
		}
	}
}

// Tile returns the pack's texture for a role, or the builtin one. Safe on a
// nil pack.
func (p *Pack) Tile(role Role) *Tile {
	if p != nil {
		if t, ok := p.tiles[role]; ok {
			return t
		}
	}
	return builtinTile(role)
}

// Fingerprint identifies the pack's contents ("builtin" when empty).
func (p *Pack) Fingerprint() string {
	if p == nil || p.fingerprint == "" {
		return "builtin"
	}
	return p.fingerprint
}

// Fingerprint stats the role files and pack.json under dir and hashes their
// names, sizes and modification times. It is cheap enough to run on every
// poll; a change re-keys every cached image.
func Fingerprint(dir string) (string, error) {
	if dir == "" {
		return "builtin", nil
	}
	names := make([]string, 0, len(AllRoles)+1)
	for _, r := range AllRoles {
		names = append(names, roleFile(r))
	}
	names = append(names, packConfigName)
	sort.Strings(names)
	h := sha256.New()
	any := false
	for _, name := range names {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return "", fmt.Errorf("texture pack: %w", err)
		}
		if fi.IsDir() {
			continue
		}
		any = true
		_, _ = fmt.Fprintf(h, "%s|%d|%d\n", name, fi.Size(), fi.ModTime().UnixNano())
	}
	if !any {
		return "builtin", nil
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}
