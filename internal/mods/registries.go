package mods

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// Registries is the ordered set of configured package registries (each a
// Thunderstore-compatible client). The first registry with id
// domain.RegistryThunderstoreID is the default; if none matches, the first
// registry is the default. A nil or empty Registries has no default and
// resolves every lookup to nil, so callers built without registries degrade
// to "package not found" rather than panicking.
type Registries struct {
	order []*Thunderstore
	byID  map[string]*Thunderstore
	def   *Thunderstore
}

// NewRegistries builds an aggregator over the given clients, in order.
func NewRegistries(clients ...*Thunderstore) *Registries {
	r := &Registries{byID: make(map[string]*Thunderstore, len(clients))}
	for _, c := range clients {
		if c == nil {
			continue
		}
		r.order = append(r.order, c)
		r.byID[c.ID()] = c
	}
	if c, ok := r.byID[domain.RegistryThunderstoreID]; ok {
		r.def = c
	} else if len(r.order) > 0 {
		r.def = r.order[0]
	}
	return r
}

// Default returns the default registry (Thunderstore when present), or nil
// when none are configured.
func (r *Registries) Default() *Thunderstore { return r.def }

// Lookup returns the registry with exactly this id. It reports false for the
// empty id, "manual", or any id that is not a configured registry.
func (r *Registries) Lookup(id string) (*Thunderstore, bool) {
	c, ok := r.byID[id]
	return c, ok
}

// GetOrDefault returns the registry with this id, falling back to the default
// for the empty or an unknown id.
func (r *Registries) GetOrDefault(id string) *Thunderstore {
	if c, ok := r.byID[id]; ok {
		return c
	}
	return r.def
}

// List returns the configured registries as API projections, in order.
func (r *Registries) List() []domain.Registry {
	out := make([]domain.Registry, 0, len(r.order))
	for _, c := range r.order {
		out = append(out, domain.Registry{ID: c.ID(), Name: c.Name()})
	}
	return out
}

// RunAll starts each registry's background refresh loop on its own goroutine,
// all bound to ctx.
func (r *Registries) RunAll(ctx context.Context) {
	for _, c := range r.order {
		go c.Run(ctx)
	}
}

// RefreshAll refreshes every registry, attempting all even if some fail, and
// returns the first error.
func (r *Registries) RefreshAll(ctx context.Context) error {
	var firstErr error
	for _, c := range r.order {
		if err := c.Refresh(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
