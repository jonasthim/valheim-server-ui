package agent

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/agent/mapstyle"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// objectsTTL is how long a fetched object list is reused before the agent
// is asked again (its own scan runs every 30 s).
const objectsTTL = 5 * time.Second

// mapInfoTTL bounds how often the plugin is asked for render progress.
const mapInfoTTL = 2 * time.Second

// MapCacheDir is where an instance's rendered maps are kept between runs.
func MapCacheDir(paths domain.InstancePaths) string {
	return filepath.Join(paths.Root, "cache", "map")
}

// mapFileName is the flat image agents before 1.10 render themselves.
func mapFileName(seed, size int) string {
	return fmt.Sprintf("map-%d-%d.png", seed, size)
}

// layersFileName is the raw layers image agents 1.10+ export.
func layersFileName(seed, size int) string {
	return fmt.Sprintf("layers-%d-%d.png", seed, size)
}

// parseLayersName recovers seed and size from a layers file name.
func parseLayersName(name string) (seed, size int, ok bool) {
	if !strings.HasPrefix(name, "layers-") || !strings.HasSuffix(name, ".png") {
		return 0, 0, false
	}
	parts := strings.Split(strings.TrimSuffix(strings.TrimPrefix(name, "layers-"), ".png"), "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	seed, err1 := strconv.Atoi(parts[0])
	size, err2 := strconv.Atoi(parts[1])
	return seed, size, err1 == nil && err2 == nil
}

// MapTexturesDir is the optional texture pack folder of an instance: PNGs
// named per mapstyle role replace the builtin textures.
func MapTexturesDir(paths domain.InstancePaths) string {
	return filepath.Join(paths.Root, "map-textures")
}

// newestCachedMap returns the most recently written cached map, or "".
func newestCachedMap(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	type cand struct {
		path string
		mod  time.Time
	}
	var c []cand
	for _, e := range entries {
		// Styled renders and the flat images of older agents count; layers,
		// masks and fog composites do not.
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		if !strings.HasPrefix(e.Name(), "map-") && !strings.HasPrefix(e.Name(), "styled-") {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		c = append(c, cand{filepath.Join(dir, e.Name()), fi.ModTime()})
	}
	if len(c) == 0 {
		return ""
	}
	sort.Slice(c, func(i, j int) bool { return c[i].mod.After(c[j].mod) })
	return c[0].path
}

// newestLayers returns the most recently written layers file, or "".
func newestLayers(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	var best string
	var bestMod time.Time
	for _, e := range entries {
		if _, _, ok := parseLayersName(e.Name()); !ok || e.IsDir() {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().After(bestMod) {
			best, bestMod = filepath.Join(dir, e.Name()), fi.ModTime()
		}
	}
	return best
}

type mapState struct {
	objects     *domain.MapObjects
	objectsAt   time.Time
	info        *domain.MapInfo
	infoAt      time.Time
	unsupported bool // the agent answered 404 to /v1/map/info

	explored       *domain.ExploredInfo
	exploredAt     time.Time
	fogUnsupported bool // 404 on /v1/map/explored/info (agent before 1.7.0)
	maskVersion    int  // exploration version of the cached mask file

	fog fogState

	tiles      tileSource     // decoded layers and mask for the tile renderer
	styleMu    sync.Mutex     // serialises styling per instance (not held under s.mu)
	fogMu      sync.Mutex     // serialises fog composites per instance (not held under s.mu)
	pack       *mapstyle.Pack // texture pack for packFP
	packFP     string
	packLogged string // fingerprint whose problems were logged
}

// fogState is the fogged composite of the cached map and mask: the image
// viewers get. It is rebuilt in the background at most every fogRebuildEvery
// while the mask keeps changing, so a 4096 px map does not cost a second of
// CPU every time someone walks a few metres.
type fogState struct {
	key      string // base and mask identity (paths and mtimes) it was built from
	path     string
	builtAt  time.Time
	building bool
}

// fogRebuildEvery bounds how often the fogged image is recomposited.
const fogRebuildEvery = 10 * time.Second

func foggedFileName(seed, size int) string {
	return fmt.Sprintf("fogmap-%d-%d.png", seed, size)
}

// foggedPathFor derives the composite's file name from a cached map's.
func foggedPathFor(basePath string) string {
	dir, name := filepath.Split(basePath)
	return filepath.Join(dir, "fog"+name)
}

func fileKey(path string) string {
	fi, err := os.Stat(path)
	if err != nil {
		return path + "|missing"
	}
	return fmt.Sprintf("%s|%d|%d", path, fi.ModTime().UnixNano(), fi.Size())
}

// exploredTTL bounds how often the fog state is asked for.
const exploredTTL = 2 * time.Second

// isNotFound reports whether err is the agent answering 404 (an older
// plugin without the map endpoints).
func isNotFound(err error) bool {
	var se *statusError
	return asStatusError(err, &se) && se.code == 404
}

// Map assembles the Map tab's data: connection, image availability, render
// progress, objects and players. includeHidden keeps hidden players'
// positions (operators).
func (s *Service) Map(ctx context.Context, id string, includeHidden bool) (*domain.InstanceMap, error) {
	in, err := s.inst.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	paths := s.inst.Paths(id)
	out := &domain.InstanceMap{MapSupported: true, Objects: []domain.MapObject{}, Pins: []domain.MapPin{}, Locations: []domain.MapLocation{}, Players: []domain.AgentPlayer{}}

	s.mu.Lock()
	x := s.states[id]
	var connected bool
	if x != nil {
		connected = x.connected
		if x.status != nil {
			w := x.status.World
			out.World = &w
			out.AgentVersion = x.status.AgentVersion
			for _, p := range x.status.Players {
				if !p.Visible && !includeHidden {
					p.Position = nil
				}
				out.Players = append(out.Players, p)
			}
		}
	}
	ms := s.maps[id]
	if ms == nil {
		ms = &mapState{}
		s.maps[id] = ms
	}
	s.mu.Unlock()
	out.Connected = connected

	if connected {
		c, err := s.client(paths, in.Config.Port)
		if err == nil {
			now := s.now()
			s.mu.Lock()
			needInfo := ms.info == nil || now.Sub(ms.infoAt) > mapInfoTTL
			needObjects := ms.objects == nil || now.Sub(ms.objectsAt) > objectsTTL
			s.mu.Unlock()
			if needInfo {
				info, err := c.MapInfo(ctx)
				s.mu.Lock()
				switch {
				case err == nil:
					ms.info, ms.infoAt, ms.unsupported = info, now, false
				case isNotFound(err):
					ms.info, ms.infoAt, ms.unsupported = nil, now, true
				}
				s.mu.Unlock()
			}
			s.mu.Lock()
			needExplored := !ms.fogUnsupported && (ms.explored == nil || now.Sub(ms.exploredAt) > exploredTTL)
			s.mu.Unlock()
			if needExplored && !ms.unsupported {
				ei, err := c.ExploredInfo(ctx)
				s.mu.Lock()
				switch {
				case err == nil:
					ms.explored, ms.exploredAt, ms.fogUnsupported = ei, now, false
				case isNotFound(err):
					ms.explored, ms.exploredAt, ms.fogUnsupported = nil, now, true
				}
				s.mu.Unlock()
			}
			if needObjects && !ms.unsupported {
				if objs, err := c.MapObjects(ctx); err == nil {
					s.mu.Lock()
					ms.objects, ms.objectsAt = objs, now
					s.mu.Unlock()
				}
			}
		}
	}

	s.mu.Lock()
	if ms.unsupported {
		out.MapSupported = false
	}
	out.LayersSupported = ms.info != nil && ms.info.Layers
	out.StyleVersion = mapstyle.Version
	out.FogSupported = !ms.fogUnsupported && !ms.unsupported
	if ms.explored != nil {
		ei := *ms.explored
		out.Explored = &ei
	}
	if ms.info != nil {
		info := *ms.info
		out.Info = &info
	}
	if ms.objects != nil {
		out.Objects = append(out.Objects, ms.objects.Objects...)
		out.Pins = append(out.Pins, ms.objects.Pins...)
		out.Locations = append(out.Locations, ms.objects.Locations...)
		out.ObjectsAt = ms.objects.UpdatedAt
	}
	s.mu.Unlock()

	// Image availability: the cached file for the current seed/size (styled
	// from layers, or an older agent's flat image), else (agent away) the
	// newest cached map from an earlier run.
	dir := MapCacheDir(paths)
	pack := s.packFor(paths, ms)
	if out.Info != nil && out.Info.Seed != 0 {
		name := mapFileName(out.Info.Seed, out.Info.Size)
		if out.Info.Layers {
			name = mapstyle.CacheName(out.Info.Seed, out.Info.Size, pack)
		}
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			out.ImageReady = true
		} else if out.Info.Layers {
			if _, err := os.Stat(filepath.Join(dir, layersFileName(out.Info.Seed, out.Info.Size))); err == nil {
				out.ImageReady = true // styled on first GET map.png
			}
		}
		if !out.ImageReady && out.Info.State == "ready" {
			out.ImageReady = true // fetched on first GET map.png
		}
	}
	if !out.ImageReady && (newestCachedMap(dir) != "" || newestLayers(dir) != "") {
		out.ImageReady = true
	}
	// Without a live agent nothing confirms the cached image matches the
	// world the server will load next (a world switch, a re-render).
	out.Stale = out.ImageReady && (!connected || out.Info == nil || out.Info.State != "ready")

	// Keep the fogged composite moving while the tab is open: the poll that
	// built this answer is what schedules the next rebuild, and its version
	// is what the browser keys the image on.
	if out.ImageReady && out.FogSupported {
		s.scheduleFog(id)
	}
	s.mu.Lock()
	out.ImageVersion = s.imageVersionLocked(ms, dir, out.Info, pack)
	out.Tiles = s.tilesLocked(ms, dir, out.Info, pack)
	s.mu.Unlock()
	return out, nil
}

// packFor returns the instance's texture pack, reloaded when the folder's
// fingerprint changes, and logs its problems once per fingerprint.
func (s *Service) packFor(paths domain.InstancePaths, ms *mapState) *mapstyle.Pack {
	dir := MapTexturesDir(paths)
	fp, err := mapstyle.Fingerprint(dir)
	if err != nil {
		s.log.Warn("agent: texture pack", "dir", dir, "err", err)
		fp = "builtin"
	}
	s.mu.Lock()
	if ms.pack != nil && ms.packFP == fp {
		p := ms.pack
		s.mu.Unlock()
		return p
	}
	s.mu.Unlock()
	pack, err := mapstyle.LoadPack(dir)
	if err != nil {
		s.log.Warn("agent: texture pack", "dir", dir, "err", err)
		pack, _ = mapstyle.LoadPack("")
	}
	s.mu.Lock()
	ms.pack, ms.packFP = pack, fp
	logIt := ms.packLogged != fp && len(pack.Problems) > 0
	ms.packLogged = fp
	s.mu.Unlock()
	if logIt {
		for _, p := range pack.Problems {
			s.log.Warn("agent: texture pack file ignored", "dir", dir, "problem", p)
		}
	}
	return pack
}

// basePathFor is the file GET map.png?fog=0 would serve for info, whether it
// exists yet or not.
func basePathFor(dir string, info *domain.MapInfo, pack *mapstyle.Pack) string {
	if info == nil || info.Seed == 0 {
		return ""
	}
	if info.Layers {
		return filepath.Join(dir, mapstyle.CacheName(info.Seed, info.Size, pack))
	}
	return filepath.Join(dir, mapFileName(info.Seed, info.Size))
}

// ensureStyled draws layersPath into the styled file for (seed, size) under
// the current style version and texture pack, unless it exists. Styling is
// serialised per instance; other versions or packs of the same seed/size are
// removed afterwards. Works offline: it needs only the layers file.
func (s *Service) ensureStyled(ctx context.Context, ms *mapState, paths domain.InstancePaths, layersPath string, seed, size int, worldRadius float32) (string, error) {
	dir := MapCacheDir(paths)
	pack := s.packFor(paths, ms)
	out := filepath.Join(dir, mapstyle.CacheName(seed, size, pack))
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	ms.styleMu.Lock()
	defer ms.styleMu.Unlock()
	if _, err := os.Stat(out); err == nil {
		return out, nil
	}
	f, err := os.Open(layersPath) //nolint:gosec // cache file the service wrote
	if err != nil {
		return "", fmt.Errorf("agent: open layers: %w", err)
	}
	layers, err := mapstyle.DecodeLayers(f)
	_ = f.Close()
	if err != nil {
		return "", fmt.Errorf("agent: %w", err)
	}
	params := mapstyle.DefaultParams(seed)
	if worldRadius > 0 {
		params.WorldRadius = worldRadius
	}
	started := s.now()
	img, err := mapstyle.Render(ctx, layers, params, pack)
	if err != nil {
		return "", fmt.Errorf("agent: style map: %w", err)
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", fmt.Errorf("agent: map cache dir: %w", err)
	}
	tmp := out + ".tmp"
	wf, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o640) //nolint:gosec // cache file
	if err != nil {
		return "", err
	}
	if err := (&png.Encoder{CompressionLevel: png.DefaultCompression}).Encode(wf, img); err != nil {
		_ = wf.Close()
		_ = os.Remove(tmp)
		return "", fmt.Errorf("agent: encode styled map: %w", err)
	}
	if err := wf.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, out); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	s.log.Info("agent: map styled", "seed", seed, "size", size, "took", s.now().Sub(started).Round(time.Millisecond), "pack", pack.Fingerprint())
	// Other styles or packs of this world are stale now.
	prefixStyled := fmt.Sprintf("styled-%d-%d-", seed, size)
	prefixFog := fmt.Sprintf("fogstyled-%d-%d-", seed, size)
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			n := e.Name()
			if (strings.HasPrefix(n, prefixStyled) || strings.HasPrefix(n, prefixFog)) && filepath.Join(dir, n) != out && !strings.HasSuffix(n, ".tmp") {
				_ = os.Remove(filepath.Join(dir, n))
			}
		}
	}
	return out, nil
}

// imageVersionLocked is a string that changes whenever GET map.png would
// serve different bytes: the base map's identity plus the fog build time.
func (s *Service) imageVersionLocked(ms *mapState, dir string, info *domain.MapInfo, pack *mapstyle.Pack) string {
	base := basePathFor(dir, info, pack)
	if base != "" {
		if _, err := os.Stat(base); err != nil {
			base = ""
		}
	}
	if base == "" {
		base = newestCachedMap(dir)
	}
	if base == "" {
		return ""
	}
	fi, err := os.Stat(base)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d-%d-v%d-%s", fi.ModTime().Unix(), ms.fog.builtAt.UnixMilli(), mapstyle.Version, pack.Fingerprint())
}

// scheduleFog rebuilds id's fogged composite in the background when the mask
// or the map changed and the last build is old enough. Returns immediately.
func (s *Service) scheduleFog(id string) {
	s.mu.Lock()
	ms := s.maps[id]
	if ms == nil || ms.fog.building || s.now().Sub(ms.fog.builtAt) < fogRebuildEvery {
		s.mu.Unlock()
		return
	}
	ms.fog.building = true
	s.mu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if _, _, err := s.foggedPNG(ctx, id, true); err != nil && !errors.Is(err, ErrMapRendering) {
			s.log.Debug("agent: fog rebuild", "instance", id, "err", err)
		}
		s.mu.Lock()
		ms.fog.building = false
		s.mu.Unlock()
	}()
}

// foggedPNG returns the fogged composite for id, building it when the base
// map or the mask changed. With wait it composites synchronously; otherwise
// a previous composite is served while a fresh one is scheduled.
func (s *Service) foggedPNG(ctx context.Context, id string, wait bool) (string, *domain.MapInfo, error) {
	base, info, err := s.basePNG(ctx, id)
	if err != nil {
		return "", info, err
	}
	maskPath, _, merr := s.ExploredPNG(ctx, id)
	if maskPath == "" {
		s.mu.Lock()
		ms := s.maps[id]
		unsupported := ms != nil && ms.fogUnsupported
		s.mu.Unlock()
		if unsupported {
			return base, info, nil // an agent without fog: nothing to hide
		}
		if errors.Is(merr, ErrMapRendering) || merr == nil {
			return "", info, ErrMapRendering // first mask still encoding; never serve the bare map
		}
		return "", info, merr
	}
	s.mu.Lock()
	ms := s.maps[id]
	if ms == nil {
		ms = &mapState{}
		s.maps[id] = ms
	}
	s.mu.Unlock()
	pack := s.packFor(s.inst.Paths(id), ms)
	key := fileKey(base) + "||" + fileKey(maskPath) + "||v" + strconv.Itoa(mapstyle.Version) + "||" + pack.Fingerprint()
	out := foggedPathFor(base)
	s.mu.Lock()
	if ms.fog.key == key && ms.fog.path != "" {
		if _, err := os.Stat(ms.fog.path); err == nil {
			s.mu.Unlock()
			return ms.fog.path, info, nil
		}
	}
	prev := ""
	if ms.fog.path != "" {
		if _, err := os.Stat(ms.fog.path); err == nil {
			prev = ms.fog.path
		}
	}
	if prev != "" && !wait {
		s.mu.Unlock()
		s.scheduleFog(id)
		return prev, info, nil
	}
	s.mu.Unlock()

	// One composite at a time per instance: a request and the background
	// rebuild must not write the same file together.
	ms.fogMu.Lock()
	defer ms.fogMu.Unlock()
	s.mu.Lock()
	if ms.fog.key == key && ms.fog.path != "" {
		if _, err := os.Stat(ms.fog.path); err == nil {
			p := ms.fog.path
			s.mu.Unlock()
			return p, info, nil
		}
	}
	s.mu.Unlock()

	seed, radius := 0, float32(0)
	if info != nil {
		seed, radius = info.Seed, float32(info.WorldRadius)
	}
	if err := writeFoggedPNG(base, maskPath, out, mapstyle.NewParchment(seed, pack), radius); err != nil {
		if prev != "" {
			return prev, info, nil
		}
		return "", info, fmt.Errorf("agent: fog composite: %w", err)
	}
	s.mu.Lock()
	ms.fog.key, ms.fog.path, ms.fog.builtAt = key, out, s.now()
	s.mu.Unlock()
	return out, info, nil
}

// ErrMapRendering is returned by MapPNG while the plugin is still rendering.
var ErrMapRendering = errors.New("map is rendering")

// MapPNG returns a local path to the map image for id. With fog (what
// every viewer gets) the image is the server-side composite of the map and
// the fog mask, so unexplored terrain never leaves the server; without fog
// it is the bare render, for operators. While the plugin renders or encodes
// its first mask it returns ErrMapRendering with the progress in info. With
// the agent away it serves the newest cached image, if any.
func (s *Service) MapPNG(ctx context.Context, id string, fog bool) (path string, info *domain.MapInfo, err error) {
	if !fog {
		return s.basePNG(ctx, id)
	}
	return s.foggedPNG(ctx, id, false)
}

// basePNG is the bare rendered map, fetched from the agent into the
// instance cache when needed.
func (s *Service) basePNG(ctx context.Context, id string) (path string, info *domain.MapInfo, err error) {
	in, err := s.inst.Get(ctx, id)
	if err != nil {
		return "", nil, err
	}
	paths := s.inst.Paths(id)
	dir := MapCacheDir(paths)

	s.mu.Lock()
	x := s.states[id]
	connected := x != nil && x.connected
	s.mu.Unlock()

	if connected {
		c, cerr := s.client(paths, in.Config.Port)
		if cerr == nil {
			mi, ierr := c.MapInfo(ctx)
			if ierr == nil {
				s.mu.Lock()
				if ms := s.maps[id]; ms != nil {
					ms.info, ms.infoAt = mi, s.now()
				} else {
					s.maps[id] = &mapState{info: mi, infoAt: s.now()}
				}
				s.mu.Unlock()
				cached := filepath.Join(dir, mapFileName(mi.Seed, mi.Size))
				fetch := c.MapPNG
				if mi.Layers {
					cached = filepath.Join(dir, layersFileName(mi.Seed, mi.Size))
					fetch = c.MapLayersPNG
				}
				switch mi.State {
				case "ready":
					if _, err := os.Stat(cached); err == nil {
						if mi.Layers {
							return s.styledFrom(ctx, id, paths, cached, mi)
						}
						return cached, mi, nil
					}
					png, _, perr := fetch(ctx)
					if perr == nil && png != nil {
						if err := os.MkdirAll(dir, 0o750); err != nil {
							return "", mi, fmt.Errorf("agent: map cache dir: %w", err)
						}
						if err := installFile(cached, png); err != nil {
							return "", mi, fmt.Errorf("agent: install map: %w", err)
						}
						if mi.Layers {
							return s.styledFrom(ctx, id, paths, cached, mi)
						}
						return cached, mi, nil
					}
				case "rendering", "encoding":
					return "", mi, ErrMapRendering
				case "idle":
					// Not started yet (auto-render off or world just loaded): kick it.
					if ri, rerr := c.RenderMap(ctx, 0, false); rerr == nil {
						return "", ri, ErrMapRendering
					}
					return "", mi, ErrMapRendering
				}
			}
		}
	}
	// Offline (or the agent has nothing yet): restyle the newest layers if a
	// world's layers are cached, else the newest styled or flat image.
	if lp := newestLayers(dir); lp != "" {
		if seed, size, ok := parseLayersName(filepath.Base(lp)); ok {
			s.mu.Lock()
			ms := s.maps[id]
			if ms == nil {
				ms = &mapState{}
				s.maps[id] = ms
			}
			s.mu.Unlock()
			if p, err := s.ensureStyled(ctx, ms, paths, lp, seed, size, 0); err == nil {
				return p, nil, nil
			}
		}
	}
	if p := newestCachedMap(dir); p != "" {
		return p, nil, nil
	}
	if !connected {
		return "", nil, domain.E(domain.CodeConflict, "no map yet: start the server with the agent installed to render one")
	}
	return "", nil, domain.E(domain.CodeConflict, "the agent has no map image yet")
}

// styledFrom styles a fetched layers file for the live world.
func (s *Service) styledFrom(ctx context.Context, id string, paths domain.InstancePaths, layersPath string, mi *domain.MapInfo) (string, *domain.MapInfo, error) {
	s.mu.Lock()
	ms := s.maps[id]
	if ms == nil {
		ms = &mapState{}
		s.maps[id] = ms
	}
	s.mu.Unlock()
	p, err := s.ensureStyled(ctx, ms, paths, layersPath, mi.Seed, mi.Size, float32(mi.WorldRadius))
	if err != nil {
		return "", mi, err
	}
	return p, mi, nil
}

// RenderMap forwards a (re)render request.
func (s *Service) RenderMap(ctx context.Context, id string, req domain.MapRenderRequest) (*domain.MapInfo, error) {
	in, err := s.inst.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	paths := s.inst.Paths(id)
	s.mu.Lock()
	x := s.states[id]
	connected := x != nil && x.connected
	s.mu.Unlock()
	if !connected {
		return nil, domain.E(domain.CodeConflict, "the agent is not connected")
	}
	c, err := s.client(paths, in.Config.Port)
	if err != nil {
		return nil, err
	}
	if req.Size != 0 && (req.Size < 256 || req.Size > 4096) {
		return nil, domain.Ef(domain.CodeValidationFailed, "size must be between 256 and 4096")
	}
	info, err := c.RenderMap(ctx, req.Size, req.Force)
	if err != nil {
		if isNotFound(err) {
			return nil, domain.E(domain.CodeConflict, "the agent running in this server has no map support yet; update it from the Mods tab (the server restarts) and try again")
		}
		return nil, domain.Ef(domain.CodeUpstreamError, "the agent refused the render: %v", err)
	}
	if req.Force {
		// The old image, layers and their styled/fogged copies are no longer wanted.
		dir := MapCacheDir(paths)
		_ = os.Remove(filepath.Join(dir, mapFileName(info.Seed, info.Size)))
		_ = os.Remove(filepath.Join(dir, layersFileName(info.Seed, info.Size)))
		if entries, err := os.ReadDir(dir); err == nil {
			ps, pf := fmt.Sprintf("styled-%d-%d-", info.Seed, info.Size), fmt.Sprintf("fogstyled-%d-%d-", info.Seed, info.Size)
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ps) || strings.HasPrefix(e.Name(), pf) {
					_ = os.Remove(filepath.Join(dir, e.Name()))
				}
			}
		}
	}
	s.mu.Lock()
	if ms := s.maps[id]; ms != nil {
		ms.info, ms.infoAt = info, s.now()
	}
	s.mu.Unlock()
	return info, nil
}

func exploredFileName(seed int) string { return fmt.Sprintf("explored-%d.png", seed) }

// ExploredPNG returns a local path to the fog mask for id, refreshing the
// instance cache when the agent reports a newer exploration version. While
// the plugin encodes its first mask it returns ErrMapRendering with the fog
// info. With the agent away the last cached mask is served.
func (s *Service) ExploredPNG(ctx context.Context, id string) (path string, info *domain.ExploredInfo, err error) {
	in, err := s.inst.Get(ctx, id)
	if err != nil {
		return "", nil, err
	}
	paths := s.inst.Paths(id)
	dir := MapCacheDir(paths)

	s.mu.Lock()
	x := s.states[id]
	connected := x != nil && x.connected
	ms := s.maps[id]
	if ms == nil {
		ms = &mapState{}
		s.maps[id] = ms
	}
	seed := 0
	if ms.info != nil {
		seed = ms.info.Seed
	}
	cachedVersion := ms.maskVersion
	s.mu.Unlock()

	if connected && !ms.fogUnsupported {
		c, cerr := s.client(paths, in.Config.Port)
		if cerr == nil {
			ei, ierr := c.ExploredInfo(ctx)
			if ierr != nil && isNotFound(ierr) {
				s.mu.Lock()
				ms.fogUnsupported = true
				s.mu.Unlock()
			} else if ierr == nil {
				s.mu.Lock()
				ms.explored, ms.exploredAt = ei, s.now()
				s.mu.Unlock()
				cached := filepath.Join(dir, exploredFileName(seed))
				if ei.MaskVersion == cachedVersion && cachedVersion != 0 {
					if _, err := os.Stat(cached); err == nil {
						return cached, ei, nil
					}
				}
				png, pinfo, perr := c.ExploredPNG(ctx)
				if perr == nil && png == nil {
					if pinfo == nil {
						pinfo = ei
					}
					if _, err := os.Stat(cached); err == nil {
						return cached, ei, nil // serve the previous mask while the new one encodes
					}
					return "", pinfo, ErrMapRendering
				}
				if perr == nil {
					if err := os.MkdirAll(dir, 0o750); err != nil {
						return "", ei, fmt.Errorf("agent: map cache dir: %w", err)
					}
					if err := installFile(cached, png); err != nil {
						return "", ei, fmt.Errorf("agent: install mask: %w", err)
					}
					s.mu.Lock()
					ms.maskVersion = ei.MaskVersion
					s.mu.Unlock()
					return cached, ei, nil
				}
			}
		}
	}
	// Offline (or unsupported): the newest cached mask, if any.
	entries, _ := os.ReadDir(dir)
	var newest string
	var newestMod time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "explored-") || !strings.HasSuffix(e.Name(), ".png") {
			continue
		}
		if fi, err := e.Info(); err == nil && fi.ModTime().After(newestMod) {
			newest, newestMod = filepath.Join(dir, e.Name()), fi.ModTime()
		}
	}
	if newest != "" {
		return newest, nil, nil
	}
	return "", nil, domain.E(domain.CodeConflict, "no exploration data yet")
}

// installFile writes data next to path under a unique temporary name and
// renames it into place, so two callers fetching the same file at once (the
// tick's fog rebuild and a request, say) never share a temp file. When the
// rename loses to another writer that already installed the file, the
// result is the same file, so that counts as success.
func installFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("cache dir: %w", err)
	}
	f, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("temp file: %w", err)
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return fmt.Errorf("write: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("close: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		if _, statErr := os.Stat(path); statErr == nil {
			return nil
		}
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
