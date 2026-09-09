package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/jobs"
	"github.com/jonasthim/valheim-server-ui/internal/mods"
)

// defaultThunderstoreRefreshHours is used when settings can't be read yet
// (ARCHITECTURE.md §12 documents the same default for
// settings.thunderstore.index_refresh_hours).
const defaultThunderstoreRefreshHours = 6

// thunderstoreHTTPTimeout bounds one index fetch or package download.
const thunderstoreHTTPTimeout = 2 * time.Minute

// wireMods constructs the Thunderstore client (WP-08), the mods service and
// the thunderstore service, attaches deps.Mods and deps.Thunderstore, and
// starts the index's background refresh loop bound to ctx.
//
// wire.go should call this from wireServices after deps.DB, deps.Bus,
// deps.Cfg, deps.Log and deps.Settings are set and after the instance
// service (WP-02) and job runner (WP-04) exist, passing them as inst/runner.
//

func wireMods(ctx context.Context, deps *api.Deps, inst *instance.Service, runner *jobs.Runner) error {
	refreshInterval := func() time.Duration {
		hours := defaultThunderstoreRefreshHours
		if deps.Settings != nil {
			if s, err := deps.Settings.Get(ctx); err == nil && s.Thunderstore.IndexRefreshHours > 0 {
				hours = s.Thunderstore.IndexRefreshHours
			}
		}
		return time.Duration(hours) * time.Hour
	}

	userAgent := fmt.Sprintf("valheim-server-ui/%s (+https://github.com/jonasthim/valheim-server-ui)", deps.Version)
	httpClient := &http.Client{Timeout: thunderstoreHTTPTimeout}

	ts := mods.NewThunderstore(httpClient, deps.Cfg.CacheDir(), refreshInterval, userAgent, deps.Log)
	go ts.Run(ctx)

	deps.Mods = mods.NewService(deps.DB, ts, inst, runner, deps.Cfg.CacheDir(), deps.Log)
	deps.Thunderstore = mods.NewThunderstoreService(ts, runner)
	return nil
}
