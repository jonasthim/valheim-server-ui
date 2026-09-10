package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

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

func mapFileName(seed, size int) string {
	return fmt.Sprintf("map-%d-%d.png", seed, size)
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
		if e.IsDir() || !strings.HasPrefix(e.Name(), "map-") || !strings.HasSuffix(e.Name(), ".png") {
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

	// Image availability: the cached file for the current seed/size, else
	// (agent away) the newest cached map from an earlier run.
	dir := MapCacheDir(paths)
	if out.Info != nil && out.Info.Seed != 0 {
		if _, err := os.Stat(filepath.Join(dir, mapFileName(out.Info.Seed, out.Info.Size))); err == nil {
			out.ImageReady = true
		} else if out.Info.State == "ready" {
			out.ImageReady = true // fetched on first GET map.png
		}
	}
	if !out.ImageReady && newestCachedMap(dir) != "" {
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
	out.ImageVersion = s.imageVersionLocked(ms, dir, out.Info)
	s.mu.Unlock()
	return out, nil
}

// imageVersionLocked is a string that changes whenever GET map.png would
// serve different bytes: the base map's identity plus the fog build time.
func (s *Service) imageVersionLocked(ms *mapState, dir string, info *domain.MapInfo) string {
	base := ""
	if info != nil && info.Seed != 0 {
		base = filepath.Join(dir, mapFileName(info.Seed, info.Size))
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
	return fmt.Sprintf("%d-%d", fi.ModTime().Unix(), ms.fog.builtAt.UnixMilli())
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
	key := fileKey(base) + "||" + fileKey(maskPath)
	out := foggedPathFor(base)
	s.mu.Lock()
	ms := s.maps[id]
	if ms == nil {
		ms = &mapState{}
		s.maps[id] = ms
	}
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

	if err := writeFoggedPNG(base, maskPath, out); err != nil {
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
				switch mi.State {
				case "ready":
					if _, err := os.Stat(cached); err == nil {
						return cached, mi, nil
					}
					png, _, perr := c.MapPNG(ctx)
					if perr == nil && png != nil {
						if err := os.MkdirAll(dir, 0o750); err != nil {
							return "", mi, fmt.Errorf("agent: map cache dir: %w", err)
						}
						tmp := cached + ".tmp"
						if err := os.WriteFile(tmp, png, 0o640); err != nil { //nolint:gosec // cache file
							return "", mi, fmt.Errorf("agent: write map: %w", err)
						}
						if err := os.Rename(tmp, cached); err != nil {
							_ = os.Remove(tmp)
							return "", mi, fmt.Errorf("agent: install map: %w", err)
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
	if p := newestCachedMap(dir); p != "" {
		return p, nil, nil
	}
	if !connected {
		return "", nil, domain.E(domain.CodeConflict, "no map yet: start the server with the agent installed to render one")
	}
	return "", nil, domain.E(domain.CodeConflict, "the agent has no map image yet")
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
		// The old image is no longer wanted.
		_ = os.Remove(filepath.Join(MapCacheDir(paths), mapFileName(info.Seed, info.Size)))
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
					tmp := cached + ".tmp"
					if err := os.WriteFile(tmp, png, 0o640); err != nil { //nolint:gosec // cache file
						return "", ei, fmt.Errorf("agent: write mask: %w", err)
					}
					if err := os.Rename(tmp, cached); err != nil {
						_ = os.Remove(tmp)
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
