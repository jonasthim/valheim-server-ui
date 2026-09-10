package agent

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/agent/mapstyle"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// The tile pyramid: zoom level z splits the world square into 2^z × 2^z
// tiles of TileSize pixels. Level 0 is one tile (82 m per pixel), level 7 is
// 32 768 px across (0.64 m per pixel, about the in-game map's deepest zoom).
// Tiles are rendered on demand from the layers, cached without fog on disk,
// and fogged per request in memory, so a fog change never invalidates the
// cache. Levels 0..prewarmZoom are rendered ahead once the layers arrive.
const (
	TileSize     = 256
	MaxZoom      = 7
	tileCacheCap = int64(1) << 30 // bytes per instance before the oldest tiles go
	pruneEvery   = 400            // tile writes between cache size checks
)

// prewarmZoom is the deepest level rendered ahead of time (341 tiles up to
// level 4). A variable so tests can shorten it.
var prewarmZoom = 4

// tileSource is what the tile renderer keeps decoded per instance.
type tileSource struct {
	layers    *mapstyle.Layers
	layersKey string
	mask      image.Image
	maskKey   string
	warmed    string // cache key base already pre-warmed
	writes    int
}

// TileDir is where an instance's tiles are cached.
func TileDir(paths domain.InstancePaths) string {
	return filepath.Join(MapCacheDir(paths), "tiles")
}

// ErrTilesUnsupported is returned for agents that do not export layers.
var ErrTilesUnsupported = domain.E(domain.CodeConflict, "the agent running in this server predates map tiles (1.10); update it from the Mods tab (the server restarts)")

// TilePNG renders (or serves from the cache) tile z/x/y for id. With fog the
// tile is composited with the current fog mask per request. The etag
// identifies the exact bytes (style, pack, tile, fog and mask version).
// While the plugin is still sampling, or before its first fog mask exists,
// it returns ErrMapRendering with the map info.
func (s *Service) TilePNG(ctx context.Context, id string, z, x, y int, fog bool) (data []byte, etag string, info *domain.MapInfo, err error) {
	if z < 0 || z > MaxZoom || x < 0 || y < 0 || x >= 1<<uint(z) || y >= 1<<uint(z) {
		return nil, "", nil, domain.E(domain.CodeNotFound, "no such tile")
	}
	layersPath, info, err := s.layersPath(ctx, id)
	if err != nil {
		return nil, "", info, err
	}
	paths := s.inst.Paths(id)
	s.mu.Lock()
	ms := s.maps[id]
	if ms == nil {
		ms = &mapState{}
		s.maps[id] = ms
	}
	s.mu.Unlock()
	pack := s.packFor(paths, ms)
	layers, params, err := s.tileLayers(ms, layersPath, info)
	if err != nil {
		return nil, "", info, err
	}
	keyBase := tileKeyBase(params.Seed, layers.Size, pack)
	s.prewarm(id, ms, keyBase)

	n := 1 << uint(z)
	ts := 2 * params.WorldRadius / float32(n)
	wx0 := -params.WorldRadius + float32(x)*ts
	wz0 := params.WorldRadius - float32(y)*ts
	wx1, wz1 := wx0+ts, wz0-ts

	tilePath := filepath.Join(TileDir(paths), keyBase, strconv.Itoa(z), strconv.Itoa(x), strconv.Itoa(y)+".png")
	var img image.Image
	if raw, rerr := os.ReadFile(tilePath); rerr == nil { //nolint:gosec // cache file the service wrote
		data = raw
	} else {
		rgba, rerr := mapstyle.RenderRegion(ctx, layers, params, pack, wx0, wz0, wx1, wz1, TileSize, TileSize)
		if rerr != nil {
			return nil, "", info, fmt.Errorf("agent: render tile: %w", rerr)
		}
		img = rgba
		var buf bytes.Buffer
		if err := (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(&buf, rgba); err != nil {
			return nil, "", info, fmt.Errorf("agent: encode tile: %w", err)
		}
		data = buf.Bytes()
		if err := writeFileAtomic(tilePath, data); err != nil {
			s.log.Debug("agent: tile cache write", "path", tilePath, "err", err)
		}
		s.mu.Lock()
		ms.tiles.writes++
		prune := ms.tiles.writes%pruneEvery == 0
		s.mu.Unlock()
		if prune {
			go pruneTileCache(TileDir(paths), tileCacheCap)
		}
	}
	etag = fmt.Sprintf("%s/%d/%d/%d", keyBase, z, x, y)
	if !fog {
		return data, `"` + etag + `"`, info, nil
	}

	mask, maskVersion, err := s.tileMask(ctx, id, ms)
	if err != nil {
		return nil, "", info, err
	}
	if mask == nil {
		// An agent without fog support: nothing to hide.
		return data, `"` + etag + `-nofog"`, info, nil
	}
	if img == nil {
		img, err = png.Decode(bytes.NewReader(data))
		if err != nil {
			return nil, "", info, fmt.Errorf("agent: decode cached tile: %w", err)
		}
	}
	fogged := compositeFogRect(img, mask, mapstyle.NewParchment(params.Seed, pack), params.WorldRadius, wx0, wz0, wx1, wz1)
	var buf bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, fogged); err != nil {
		return nil, "", info, fmt.Errorf("agent: encode fogged tile: %w", err)
	}
	return buf.Bytes(), fmt.Sprintf(`"%s-m%d"`, etag, maskVersion), info, nil
}

// TilesFor describes the pyramid for InstanceMap.tiles, or nil when the
// running agent does not export layers and no layers are cached.
func (s *Service) tilesLocked(ms *mapState, dir string, info *domain.MapInfo, pack *mapstyle.Pack) *domain.MapTiles {
	seed, size := 0, 0
	switch {
	case info != nil && info.Layers && info.Seed != 0:
		seed, size = info.Seed, info.Size
	default:
		lp := newestLayers(dir)
		if lp == "" {
			return nil
		}
		var ok bool
		if seed, size, ok = parseLayersName(filepath.Base(lp)); !ok {
			return nil
		}
	}
	return &domain.MapTiles{
		TileSize: TileSize,
		MaxZoom:  MaxZoom,
		Version:  fmt.Sprintf("%s-m%d", tileKeyBase(seed, size, pack), ms.maskVersion),
	}
}

func tileKeyBase(seed, size int, pack *mapstyle.Pack) string {
	return fmt.Sprintf("%d-%d-v%d-%s", seed, size, mapstyle.Version, pack.Fingerprint())
}

// layersPath locates the layers file for id, fetching (and styling, which
// map.png needs anyway) through basePNG when the agent is live. Agents
// without layers are reported as unsupported.
func (s *Service) layersPath(ctx context.Context, id string) (string, *domain.MapInfo, error) {
	paths := s.inst.Paths(id)
	dir := MapCacheDir(paths)
	_, info, err := s.basePNG(ctx, id)
	if err != nil {
		return "", info, err
	}
	if info != nil {
		if !info.Layers {
			return "", info, ErrTilesUnsupported
		}
		p := filepath.Join(dir, layersFileName(info.Seed, info.Size))
		if _, err := os.Stat(p); err == nil {
			return p, info, nil
		}
	}
	if lp := newestLayers(dir); lp != "" {
		return lp, info, nil
	}
	if info != nil {
		return "", info, ErrMapRendering
	}
	return "", nil, ErrTilesUnsupported
}

// tileLayers returns the decoded layers for the file, cached per instance
// until the file changes, and the render params.
func (s *Service) tileLayers(ms *mapState, layersPath string, info *domain.MapInfo) (*mapstyle.Layers, mapstyle.Params, error) {
	key := fileKey(layersPath)
	ms.styleMu.Lock()
	defer ms.styleMu.Unlock()
	if ms.tiles.layers == nil || ms.tiles.layersKey != key {
		f, err := os.Open(layersPath) //nolint:gosec // cache file the service wrote
		if err != nil {
			return nil, mapstyle.Params{}, fmt.Errorf("agent: open layers: %w", err)
		}
		l, err := mapstyle.DecodeLayers(f)
		_ = f.Close()
		if err != nil {
			return nil, mapstyle.Params{}, fmt.Errorf("agent: %w", err)
		}
		ms.tiles.layers, ms.tiles.layersKey = l, key
	}
	seed := 0
	radius := float32(0)
	if info != nil {
		seed, radius = info.Seed, float32(info.WorldRadius)
	} else if sd, _, ok := parseLayersName(filepath.Base(layersPath)); ok {
		seed = sd
	}
	params := mapstyle.DefaultParams(seed)
	if radius > 0 {
		params.WorldRadius = radius
	}
	return ms.tiles.layers, params, nil
}

// tileMask returns the decoded fog mask (nil for agents without fog) and
// the version of the mask file it came from.
func (s *Service) tileMask(ctx context.Context, id string, ms *mapState) (image.Image, int, error) {
	maskPath, _, merr := s.ExploredPNG(ctx, id)
	if maskPath == "" {
		s.mu.Lock()
		unsupported := ms.fogUnsupported
		s.mu.Unlock()
		if unsupported {
			return nil, 0, nil
		}
		if merr == nil || errors.Is(merr, ErrMapRendering) {
			return nil, 0, ErrMapRendering
		}
		return nil, 0, merr
	}
	key := fileKey(maskPath)
	ms.fogMu.Lock()
	defer ms.fogMu.Unlock()
	if ms.tiles.mask == nil || ms.tiles.maskKey != key {
		m, err := decodePNG(maskPath)
		if err != nil {
			return nil, 0, fmt.Errorf("agent: fog mask: %w", err)
		}
		ms.tiles.mask, ms.tiles.maskKey = m, key
	}
	s.mu.Lock()
	v := ms.maskVersion
	s.mu.Unlock()
	return ms.tiles.mask, v, nil
}

// prewarm renders levels 0..prewarmZoom in the background once per cache
// key, so the first view of a world is instant.
func (s *Service) prewarm(id string, ms *mapState, keyBase string) {
	s.mu.Lock()
	if ms.tiles.warmed == keyBase {
		s.mu.Unlock()
		return
	}
	ms.tiles.warmed = keyBase
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		for z := 0; z <= prewarmZoom; z++ {
			n := 1 << uint(z)
			for y := 0; y < n; y++ {
				for x := 0; x < n; x++ {
					if ctx.Err() != nil {
						return
					}
					if _, _, _, err := s.TilePNG(ctx, id, z, x, y, false); err != nil {
						s.log.Debug("agent: tile prewarm stopped", "instance", id, "err", err)
						return
					}
				}
			}
		}
		pruneTileCache(TileDir(s.inst.Paths(id)), tileCacheCap)
	}()
}

// pruneTileCache deletes the least recently modified tiles until the folder
// is under cap (with 20 % headroom).
func pruneTileCache(dir string, cap int64) {
	type f struct {
		path string
		size int64
		mod  time.Time
	}
	var files []f
	var total int64
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if fi, err := d.Info(); err == nil {
			files = append(files, f{path, fi.Size(), fi.ModTime()})
			total += fi.Size()
		}
		return nil
	})
	if total <= cap {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mod.Before(files[j].mod) })
	target := cap * 8 / 10
	for _, e := range files {
		if total <= target {
			break
		}
		if err := os.Remove(e.path); err == nil {
			total -= e.size
		}
	}
}

// writeFileAtomic writes data to path through a uniquely named temp file, so
// two renders of the same tile cannot corrupt each other.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	var nonce [4]byte
	_, _ = rand.Read(nonce[:])
	tmp := path + ".tmp-" + hex.EncodeToString(nonce[:])
	if err := os.WriteFile(tmp, data, 0o640); err != nil { //nolint:gosec // cache file
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
