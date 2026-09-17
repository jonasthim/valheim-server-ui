package metrics

import (
	"context"
	"time"

	"github.com/jonasthim/valheim-server-ui/internal/domain"
)

// sampleRanger is the read side of SampleStore (internal/db.MetricSamplesRepo).
type sampleRanger interface {
	Range(ctx context.Context, instanceID *string, since time.Time) ([]domain.MetricSample, error)
}

// History implements api.MetricsHistoryService: it maps stored samples to the
// parallel-array MetricSeries shape the API returns.
type History struct {
	store sampleRanger
}

// NewHistory builds a History reading from store.
func NewHistory(store sampleRanger) *History {
	return &History{store: store}
}

// Series returns instanceID's samples (nil = the host) since the given time,
// ascending, as parallel arrays. The result's slices are never nil, even when
// there are no samples in range.
func (h *History) Series(ctx context.Context, instanceID *string, since time.Time) (domain.MetricSeries, error) {
	samples, err := h.store.Range(ctx, instanceID, since)
	if err != nil {
		return domain.MetricSeries{}, err
	}
	out := domain.MetricSeries{
		TS:       make([]string, 0, len(samples)),
		CPU:      make([]float64, 0, len(samples)),
		Mem:      make([]int64, 0, len(samples)),
		Players:  make([]int, 0, len(samples)),
		DiskFree: make([]int64, 0, len(samples)),
	}
	for _, s := range samples {
		out.TS = append(out.TS, s.At.UTC().Format(time.RFC3339))
		out.CPU = append(out.CPU, s.CPU)
		out.Mem = append(out.Mem, s.Mem)
		out.Players = append(out.Players, s.Players)
		out.DiskFree = append(out.DiskFree, s.DiskFree)
	}
	return out, nil
}
