package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/agent"
	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/domain"
	"github.com/jonasthim/valheim-server-ui/internal/instance"
	"github.com/jonasthim/valheim-server-ui/internal/selfupdate"
)

// wireAgent sets up the Valheim UI Agent integration: the bundles that
// provide the agent and gameplay plugin packages (installed with BepInEx by
// the mods service), the poller that talks to running agents, the status
// enricher and the pre-start hook that writes the plugin's config. Returns
// both bundles for wireMods.
func wireAgent(ctx context.Context, deps *api.Deps, inst *instance.Service) (agentBundle, gameplayBundle *agent.Bundle) {
	var copts []selfupdate.ClientOption
	if base := os.Getenv("VALHEIM_UI_RELEASES_BASE_URL"); base != "" {
		copts = append(copts, selfupdate.WithBaseURL(base))
	}
	releases := selfupdate.NewClient(domain.GitHubRepo, version, copts...)
	hc := &http.Client{Timeout: 2 * time.Minute}
	agentBundle = agent.NewBundle(domain.BundledAgent, version, deps.Cfg.CacheDir(), releases, hc)
	gameplayBundle = agent.NewBundle(domain.BundledGameplay, version, deps.Cfg.CacheDir(), releases, hc)

	svc := agent.NewService(inst, deps.Bus, deps.Log, agentBundle)
	// F-2.3: persistent chat history, stored by the poller and served back
	// through deps.Agent.ChatHistory.
	svc.SetChatStore(db.NewChatLogRepo(deps.DB))
	svc.SetSurvivalStore(db.NewSurvivalRepo(deps.DB))
	inst.RegisterEnricher(svc)
	inst.RegisterPreStart(svc.PreStart)
	deps.Agent = svc
	deps.Survival = svc
	go svc.Run(ctx)
	return agentBundle, gameplayBundle
}
