package main

import (
	"context"

	"github.com/jonasthim/valheim-server-ui/internal/api"
	"github.com/jonasthim/valheim-server-ui/internal/db"
	"github.com/jonasthim/valheim-server-ui/internal/metrics"
)

// wireMetrics constructs the metric_samples repo and the F-1.3 history
// service, attaches deps.MetricsHistory, and starts the background sample
// recorder bound to ctx.
//
// wire.go calls this at the end of wireServices, after deps.Instances (WP-02)
// and deps.Metrics (the host sampler, set right after wireInstances) both
// exist.
func wireMetrics(ctx context.Context, deps *api.Deps) error {
	repo := db.NewMetricSamplesRepo(deps.DB)
	deps.MetricsHistory = metrics.NewHistory(repo)

	rec := metrics.NewRecorder(repo, deps.Instances, hostOrNil(deps.Metrics), deps.Cfg.DataDir, deps.Log)
	go rec.Run(ctx)
	return nil
}

// hostOrNil adapts deps.Metrics (api.HostMetricsSource, populated by
// internal/metrics.Sampler but nil-able) to metrics.HostSource, passing a
// literal nil rather than a typed-nil interface when it is unset.
func hostOrNil(m api.HostMetricsSource) metrics.HostSource {
	if m == nil {
		return nil
	}
	return m
}
