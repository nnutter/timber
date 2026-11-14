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
	fetch    func(context.Context) (Report, error)
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
	results := make([]Report, len(c.fetchers))
	keep := make([]bool, len(c.fetchers))
	var group sync.WaitGroup
	for index, item := range c.fetchers {
		group.Go(func() {
			report, err := item.fetch(ctx)
			kept, ok := guard(item.id, item.name, report, err)
			results[index] = kept
			keep[index] = ok
		})
	}
	group.Wait()
	if err := ctx.Err(); err != nil {
		return Snapshot{}, err
	}
	reports := make([]Report, 0, len(results))
	for index, report := range results {
		if keep[index] {
			reports = append(reports, report)
		}
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

func guard(id, name string, report Report, err error) (Report, bool) {
	if err == nil {
		report.ID = id
		if report.Name == "" {
			report.Name = name
		}
		report.OK = true
		report.Limits = Standing(report.Limits)
		return report, true
	}
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

func defaultFetchers(source Source, client *http.Client) []fetcher {
	return []fetcher{
		{anthropicID, anthropicName, func(ctx context.Context) (Report, error) {
			return fetchAnthropic(ctx, source, client)
		}},
		{copilotID, copilotName, func(ctx context.Context) (Report, error) {
			return fetchCopilot(ctx, source, client)
		}},
		{codexID, codexName, func(ctx context.Context) (Report, error) {
			return fetchCodex(ctx, source, client)
		}},
		{openCodeID, openCodeName, func(ctx context.Context) (Report, error) {
			return fetchOpenCode(ctx, source, client)
		}},
		{grokID, grokName, func(ctx context.Context) (Report, error) {
			return fetchGrok(ctx, source, client)
		}},
		{zaiID, zaiName, func(ctx context.Context) (Report, error) {
			return fetchZAI(ctx, source, client)
		}},
		{miniMaxID, miniMaxName, func(ctx context.Context) (Report, error) {
			return fetchMiniMax(ctx, source, client)
		}},
		{deepSeekID, deepSeekName, func(ctx context.Context) (Report, error) {
			return fetchDeepSeek(ctx, source, client)
		}},
		{ollamaID, ollamaName, func(ctx context.Context) (Report, error) {
			return fetchOllama(ctx, source, client)
		}},
		{museID, museName, func(ctx context.Context) (Report, error) {
			return fetchMuse(ctx, source, client)
		}},
	}
}

func withinCache(age time.Duration) bool { return age < freshFor }
