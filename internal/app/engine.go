// Package app orchestrates probing, classification and history (ADR 0001).
package app

import (
	"context"
	"log/slog"
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
}

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

	state     domain.State
	last      *domain.Outage
	announced int64                   // ID of the open outage already logged
	pending   map[int64]domain.Outage // outages not saved yet, retried every tick
}

// NewEngine wires the engine to its ports.
func NewEngine(p Prober, s Store, sink Sink, t *domain.Tracker, interval time.Duration, log *slog.Logger) *Engine {
	return &Engine{
		prober: p, store: s, sink: sink, tracker: t,
		interval: interval, log: log,
		pending: make(map[int64]domain.Outage),
	}
}

// Run probes until ctx is cancelled. A failing store never stops probing: the
// outage is kept in memory and saved again on the next tick.
func (e *Engine) Run(ctx context.Context) error {
	last, err := e.store.Last(ctx)
	if err != nil {
		e.log.Error("read last outage", "err", err)
	}
	e.last = last

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		e.tick(ctx)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
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
