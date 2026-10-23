package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/domain"
)

var t0 = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

type scriptedProber struct {
	samples []domain.Sample
	next    int
}

func (p *scriptedProber) Probe(context.Context) domain.Sample {
	s := p.samples[p.next]
	p.next++
	return s
}

type memStore struct {
	saved   map[int64]domain.Outage
	failing bool
	last    *domain.Outage
}

func (s *memStore) Save(_ context.Context, o domain.Outage) error {
	if s.failing {
		return errors.New("disk full")
	}
	s.saved[o.ID] = o
	return nil
}

func (s *memStore) Last(context.Context) (*domain.Outage, error) { return s.last, nil }

type captureSink struct{ statuses []Status }

func (c *captureSink) Publish(s Status) { c.statuses = append(c.statuses, s) }

func sample(sec int, internet bool) domain.Sample {
	return domain.Sample{At: t0.Add(time.Duration(sec) * time.Second), Link: true, Router: true, Internet: internet, DNS: internet}
}

func newEngine(p Prober, s Store, sink Sink) *Engine {
	tracker := domain.NewTracker(domain.TrackerConfig{FailThreshold: 3, MergeGap: 10 * time.Second})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewEngine(p, s, sink, tracker, 2*time.Second, log)
}

func TestEngineStoresOutageAndPublishesStatus(t *testing.T) {
	prober := &scriptedProber{samples: []domain.Sample{
		sample(0, true), sample(2, false), sample(4, false), sample(6, false), sample(8, true),
	}}
	store := &memStore{saved: map[int64]domain.Outage{}}
	sink := &captureSink{}
	e := newEngine(prober, store, sink)

	for range prober.samples {
		e.tick(context.Background())
	}

	if len(store.saved) != 1 {
		t.Fatalf("saved outages = %d, want 1", len(store.saved))
	}
	for _, o := range store.saved {
		if o.Open() || o.Cause != domain.CauseLine || o.Duration() != 6*time.Second {
			t.Fatalf("saved outage = %+v, want closed line outage of 6s", o)
		}
	}
	states := make([]domain.State, len(sink.statuses))
	for i, s := range sink.statuses {
		states[i] = s.State
	}
	want := []domain.State{domain.StateUp, domain.StateDegraded, domain.StateDegraded, domain.StateDown, domain.StateUp}
	for i := range want {
		if states[i] != want[i] {
			t.Fatalf("published states = %v, want %v", states, want)
		}
	}
	if last := sink.statuses[len(sink.statuses)-1].Last; last == nil || last.Open() {
		t.Fatalf("last published outage = %+v, want the closed one", last)
	}
}

func TestEngineRetriesFailedSaveWithoutStopping(t *testing.T) {
	prober := &scriptedProber{samples: []domain.Sample{
		sample(0, false), sample(2, false), sample(4, false), // opens the outage while the store fails
		sample(6, true), // closes it, the store is back
	}}
	store := &memStore{saved: map[int64]domain.Outage{}, failing: true}
	e := newEngine(prober, store, &captureSink{})

	for i := 0; i < 3; i++ {
		e.tick(context.Background())
	}
	if len(store.saved) != 0 {
		t.Fatalf("saved = %d while the store fails, want 0", len(store.saved))
	}

	store.failing = false
	e.tick(context.Background())
	if len(store.saved) != 1 {
		t.Fatalf("saved = %d after recovery, want 1", len(store.saved))
	}
	for _, o := range store.saved {
		if o.Open() {
			t.Fatalf("saved outage = %+v, want the closed version", o)
		}
	}
}

func TestEngineRunStartsFromLastStoredOutage(t *testing.T) {
	stored := &domain.Outage{ID: 1, Start: t0.Add(-time.Hour), End: t0.Add(-time.Hour + 5*time.Second), Cause: domain.CauseRouter}
	store := &memStore{saved: map[int64]domain.Outage{}, last: stored}
	sink := &captureSink{}
	prober := &scriptedProber{samples: []domain.Sample{sample(0, true)}}
	e := newEngine(prober, store, sink)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := e.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() = %v, want context.Canceled", err)
	}
	if e.last != stored {
		t.Fatalf("engine last outage = %+v, want the stored one", e.last)
	}
}
