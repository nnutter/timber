package quotas

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"
)

const (
	freshFor = time.Minute
	minGap   = 10 * time.Second
)

type fetcher struct {
	id, name string
	fetch    func(context.Context) ([]Report, error)
}

// Collector shares one read of every service.
// A fresh answer is kept for a minute. A forced read still waits at least ten seconds.
type Collector struct {
	source   Source
	client   *http.Client
	fetchers []fetcher
	now      func() time.Time

	mu       sync.Mutex
	have     bool
	fetched  time.Time
	snapshot Snapshot
}

// Reports returns the last answer while it is fresh, or a new read of every service.
// fresh asks the services again, still no more often than the minimum gap.
func (c *Collector) Reports(ctx context.Context, fresh bool) (Snapshot, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock()
	if c.have {
		age := now.Sub(c.fetched)
		if age < minGap || (!fresh && withinCache(age)) {
			return c.snapshot, nil
		}
	}
	snapshot, err := c.read(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	c.fetched = c.clock()
	c.have = true
	c.snapshot = snapshot
	return snapshot, nil
}

func (c *Collector) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

func (c *Collector) read(ctx context.Context) (Snapshot, error) {
	results := make([][]Report, len(c.fetchers))
	var group sync.WaitGroup
	for index, item := range c.fetchers {
		group.Go(func() {
			reports, err := item.fetch(ctx)
			results[index] = guard(item.id, item.name, reports, err)
		})
	}
	group.Wait()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	reports := make([]Report, 0, len(results))
	for _, batch := range results {
		reports = append(reports, batch...)
	}
	return Snapshot{FetchedAt: c.clock().UTC(), Reports: reports}, nil
}

// NewCollector reads every service the Hermes quotas panel reads.
func NewCollector() *Collector {
	source := Source{}
	client := NewHTTPClient()
	return &Collector{
		source:   source,
		client:   client,
		fetchers: defaultFetchers(source, client),
	}
}

func guard(id, name string, reports []Report, err error) []Report {
	if err != nil {
		if failed, ok := errorReport(id, name, err); ok {
			return []Report{failed}
		}
		return nil
	}
	kept := make([]Report, 0, len(reports))
	for _, report := range reports {
		kept = append(kept, normalizeReport(id, name, report))
	}
	return kept
}

func normalizeReport(id, name string, report Report) Report {
	if report.ID == "" {
		report.ID = id
	}
	if report.Name == "" {
		report.Name = name
	}
	report.OK = true
	report.Limits = Standing(report.Limits)
	return report
}

func errorReport(id, name string, err error) (Report, bool) {
	if _, ok := errors.AsType[MissingError](err); ok {
		return Report{}, false
	}
	if status, ok := errors.AsType[StatusError](err); ok {
		return Report{ID: id, Name: name, Error: status.Error()}, true
	}
	if said, ok := errors.AsType[SaidError](err); ok {
		return Report{ID: id, Name: name, Error: said.Error()}, true
	}
	return Report{ID: id, Name: name, Error: readFailure(err)}, true
}

func singleFetcher(id, name string, fetch func(context.Context) (Report, error)) fetcher {
	return fetcher{
		id:   id,
		name: name,
		fetch: func(ctx context.Context) ([]Report, error) {
			report, err := fetch(ctx)
			if err != nil {
				return nil, err
			}
			return []Report{report}, nil
		},
	}
}

func defaultFetchers(source Source, client *http.Client) []fetcher {
	return []fetcher{
		singleFetcher(anthropicID, anthropicName, func(ctx context.Context) (Report, error) {
			return fetchAnthropic(ctx, source, client)
		}),
		singleFetcher(copilotID, copilotName, func(ctx context.Context) (Report, error) {
			return fetchCopilot(ctx, source, client)
		}),
		singleFetcher(codexID, codexName, func(ctx context.Context) (Report, error) {
			return fetchCodex(ctx, source, client)
		}),
		singleFetcher(openCodeID, openCodeName, func(ctx context.Context) (Report, error) {
			return fetchOpenCode(ctx, source, client)
		}),
		singleFetcher(grokID, grokName, func(ctx context.Context) (Report, error) {
			return fetchGrok(ctx, source, client)
		}),
		singleFetcher(zaiID, zaiName, func(ctx context.Context) (Report, error) {
			return fetchZAI(ctx, source, client)
		}),
		singleFetcher(miniMaxID, miniMaxName, func(ctx context.Context) (Report, error) {
			return fetchMiniMax(ctx, source, client)
		}),
		singleFetcher(deepSeekID, deepSeekName, func(ctx context.Context) (Report, error) {
			return fetchDeepSeek(ctx, source, client)
		}),
		singleFetcher(ollamaID, ollamaName, func(ctx context.Context) (Report, error) {
			return fetchOllama(ctx, source, client)
		}),
		singleFetcher(museID, museName, func(ctx context.Context) (Report, error) {
			return fetchMuse(ctx, source, client)
		}),
		{id: googleID, name: googleName, fetch: func(ctx context.Context) ([]Report, error) {
			return fetchGoogle(ctx, source, client)
		}},
	}
}

func withinCache(age time.Duration) bool { return age < freshFor }
