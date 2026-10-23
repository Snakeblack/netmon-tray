// Package app orchestrates probing, classification and history (ADR 0001).
package app

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/domain"
)

// Prober measures every layer once.
type Prober interface {
	Probe(ctx context.Context) domain.Sample
}

// Store persists outages.
type Store interface {
	Save(ctx context.Context, o domain.Outage) error
	Last(ctx context.Context) (*domain.Outage, error)
	List(ctx context.Context) ([]domain.Outage, error)
	// Purge deletes the outages that ended before the given instant.
	Purge(ctx context.Context, before time.Time) (int64, error)
}

// Exporter writes the history to files and returns their paths.
type Exporter interface {
	Write(outages []domain.Outage) ([]string, error)
}

// purgeEvery is how often the engine drops outages past the retention.
const purgeEvery = 24 * time.Hour

// Sink receives the status every time a sample is processed.
type Sink interface {
	Publish(Status)
}

// Status is the state shown to the user plus the last known outage.
type Status struct {
	domain.Status
	Last *domain.Outage
}

// Engine probes on a fixed interval, tracks outages and stores them.
type Engine struct {
	prober   Prober
	store    Store
	sink     Sink
	tracker  *domain.Tracker
	interval time.Duration
	log      *slog.Logger

	retention time.Duration // zero keeps the history forever
	exporter  Exporter
	exportMu  sync.Mutex // the tray and ctl may export at the same time
	now       func() time.Time

	state     domain.State
	last      *domain.Outage
	announced int64                   // ID of the open outage already logged
	pending   map[int64]domain.Outage // outages not saved yet, retried every tick
}

// Option customises an Engine.
type Option func(*Engine)

// WithRetention makes the engine drop outages that ended more than d ago.
func WithRetention(d time.Duration) Option { return func(e *Engine) { e.retention = d } }

// WithExporter lets the engine export its history.
func WithExporter(x Exporter) Option { return func(e *Engine) { e.exporter = x } }

// NewEngine wires the engine to its ports.
func NewEngine(p Prober, s Store, sink Sink, t *domain.Tracker, interval time.Duration, log *slog.Logger, opts ...Option) *Engine {
	e := &Engine{
		prober: p, store: s, sink: sink, tracker: t,
		interval: interval, log: log,
		now:     time.Now,
		pending: make(map[int64]domain.Outage),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Run probes until ctx is cancelled. A failing store never stops probing: the
// outage is kept in memory and saved again on the next tick.
func (e *Engine) Run(ctx context.Context) error {
	last, err := e.store.Last(ctx)
	if err != nil {
		e.log.Error("read last outage", "err", err)
	}
	e.last = last
	e.purge(ctx)

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	purgeTicker := time.NewTicker(purgeEvery)
	defer purgeTicker.Stop()
	for {
		e.tick(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-purgeTicker.C:
			e.purge(ctx)
		case <-ticker.C:
		}
	}
}

// Export writes the stored history to files and returns their paths. It is
// safe to call while the engine runs; concurrent calls take turns.
func (e *Engine) Export(ctx context.Context) ([]string, error) {
	e.exportMu.Lock()
	defer e.exportMu.Unlock()
	if e.exporter == nil {
		return nil, errors.New("export is not configured")
	}
	outages, err := e.store.List(ctx)
	if err != nil {
		return nil, err
	}
	return e.exporter.Write(outages)
}

// purge drops the outages past the retention. A failure is only logged: the
// next run retries it and probing goes on.
func (e *Engine) purge(ctx context.Context) {
	if e.retention <= 0 {
		return
	}
	removed, err := e.store.Purge(ctx, e.now().Add(-e.retention))
	if err != nil {
		e.log.Error("purge history", "err", err)
		return
	}
	if removed > 0 {
		e.log.Info("history purged", "outages", removed)
	}
}

func (e *Engine) tick(ctx context.Context) {
	sample := e.prober.Probe(ctx)
	if ctx.Err() != nil {
		return
	}
	status, outage := e.tracker.Observe(sample)
	if outage != nil {
		e.pending[outage.ID] = *outage
		e.last = outage
		e.logOutage(*outage)
	}
	e.flush(ctx)
	if status.State != e.state {
		e.state = status.State
		e.log.Info("state changed", "state", status.State.String(), "cause", status.Cause)
	}
	e.sink.Publish(Status{Status: status, Last: e.last})
}

func (e *Engine) flush(ctx context.Context) {
	for id, o := range e.pending {
		if err := e.store.Save(ctx, o); err != nil {
			e.log.Error("save outage", "id", id, "err", err)
			continue
		}
		delete(e.pending, id)
	}
}

// logOutage logs only the transitions, not every sample of an open outage.
func (e *Engine) logOutage(o domain.Outage) {
	switch {
	case !o.Open():
		e.announced = 0
		e.log.Info("outage closed", "cause", o.Cause, "duration", o.Duration().Round(time.Second))
	case o.ID != e.announced:
		e.announced = o.ID
		e.log.Warn("outage opened", "cause", o.Cause, "since", o.Start)
	}
}
