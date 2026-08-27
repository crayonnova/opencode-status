package poller

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/nova/opencode-status/internal/fetcher"
	"github.com/nova/opencode-status/internal/storage"
)

type Poller struct {
	Store          *storage.Store
	Fetcher        *fetcher.Fetcher
	Interval       time.Duration
	IncludePaid    bool
	Logger         *log.Logger
}

type Snapshot struct {
	At       time.Time
	Models   []storage.ModelState
	Uptimes  map[string]float64
	Samples  map[string]int
}

func (p *Poller) Run(ctx context.Context) error {
	// Initial tick immediately, then on Interval.
	if err := p.tick(ctx); err != nil {
		p.logf("initial tick error: %v", err)
	}
	t := time.NewTicker(p.Interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			if err := p.tick(ctx); err != nil {
				p.logf("tick error: %v", err)
			}
		}
	}
}

func (p *Poller) tick(ctx context.Context) error {
	// Use OpenRouter as the source of truth for "currently available free models".
	// Without a key, free-tier calls may be rate-limited; we still want catalog visibility.
	models, err := p.Fetcher.FetchFromOpenRouter(ctx, p.IncludePaid)
	if err != nil {
		return err
	}

	// Sync model registry.
	for _, m := range models {
		if err := p.Store.UpsertModel(m.ID, m.Provider, m.Name, m.IsFree); err != nil {
			return err
		}
		if err := p.Store.RecordCheck(m.ID, m.IsFree, time.Now()); err != nil {
			return err
		}
	}
	p.logf("recorded %d models (free=%d)", len(models), countFree(models))
	return nil
}

func (p *Poller) logf(format string, args ...any) {
	if p.Logger != nil {
		p.Logger.Printf(format, args...)
	}
}

func countFree(ms []fetcher.Model) int {
	n := 0
	for _, m := range ms {
		if m.IsFree {
			n++
		}
	}
	return n
}

// Snapshot returns the current model list with uptime% over the last `window`.
func SnapshotFromStore(store *storage.Store, window time.Duration) (*Snapshot, error) {
	models, err := store.AllModels()
	if err != nil {
		return nil, err
	}
	since := time.Now().Add(-window)

	type up struct {
		f    float64
		n    int
	}
	uptimes := make(map[string]float64, len(models))
	samples := make(map[string]int, len(models))

	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, m := range models {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			f, n, err := store.UptimeFraction(id, since)
			if err != nil {
				return
			}
			mu.Lock()
			uptimes[id] = f
			samples[id] = n
			mu.Unlock()
		}(m.ModelID)
	}
	wg.Wait()

	sort.Slice(models, func(i, j int) bool {
		if models[i].IsFree != models[j].IsFree {
			return models[i].IsFree
		}
		if models[i].ProviderID != models[j].ProviderID {
			return models[i].ProviderID < models[j].ProviderID
		}
		return models[i].Name < models[j].Name
	})
	return &Snapshot{At: time.Now(), Models: models, Uptimes: uptimes, Samples: samples}, nil
}
