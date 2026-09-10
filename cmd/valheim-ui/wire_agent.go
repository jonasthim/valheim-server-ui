package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/agent"
	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/selfupdate"
)

// wireAgent sets up the Valheim UI Agent integration: the bundle that
// provides the plugin package (installed with BepInEx by the mods service),
// the poller that talks to running agents, the status enricher and the
// pre-start hook that writes the plugin's config. Returns the bundle for
// wireMods.
func wireAgent(ctx context.Context, deps *api.Deps, inst *instance.Service) *agent.Bundle {
	var copts []selfupdate.ClientOption
	if base := os.Getenv("VALHEIM_UI_RELEASES_BASE_URL"); base != "" {
		copts = append(copts, selfupdate.WithBaseURL(base))
	}
	releases := selfupdate.NewClient(domain.GitHubRepo, version, copts...)
	bundle := agent.NewBundle(version, deps.Cfg.CacheDir(), releases, &http.Client{Timeout: 2 * time.Minute})

	svc := agent.NewService(inst, deps.Bus, deps.Log, bundle)
	inst.RegisterEnricher(svc)
	inst.RegisterPreStart(svc.PreStart)
	deps.Agent = svc
	go svc.Run(ctx)
	return bundle
}
