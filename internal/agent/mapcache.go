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
const objectsTTL = 10 * time.Second

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
}

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
	out := &domain.InstanceMap{MapSupported: true, Objects: []domain.MapObject{}, Locations: []domain.MapLocation{}, Players: []domain.AgentPlayer{}}

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
	if ms.info != nil {
		info := *ms.info
		out.Info = &info
	}
	if ms.objects != nil {
		out.Objects = append(out.Objects, ms.objects.Objects...)
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
	return out, nil
}

// ErrMapRendering is returned by MapPNG while the plugin is still rendering.
var ErrMapRendering = errors.New("map is rendering")

// MapPNG returns a local path to the map image for id, fetching it from the
// agent into the instance cache when needed. While the plugin renders it
// returns ErrMapRendering with the progress in info. With the agent away it
// serves the newest cached image, if any.
func (s *Service) MapPNG(ctx context.Context, id string) (path string, info *domain.MapInfo, err error) {
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
