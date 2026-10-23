package domain

import (
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

func at(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }

func sample(sec int, link, router, internet, dns bool) Sample {
	return Sample{At: at(sec), Link: link, Router: router, Internet: internet, DNS: dns}
}

func TestClassifyBlamesClosestFailingLayer(t *testing.T) {
	tests := []struct {
		name       string
		s          Sample
		wantOutage bool
		wantCause  Cause
	}{
		{"all up", sample(0, true, true, true, true), false, CauseNone},
		{"link down hides everything beyond", sample(0, false, false, false, false), true, CauseLink},
		{"router down", sample(0, true, false, false, false), true, CauseRouter},
		{"router ok but internet down", sample(0, true, true, false, false), true, CauseLine},
		{"only dns down is not an outage", sample(0, true, true, true, false), false, CauseNone},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotOutage, gotCause := Classify(tt.s)
			if gotOutage != tt.wantOutage || gotCause != tt.wantCause {
				t.Fatalf("Classify() = (%v, %q), want (%v, %q)", gotOutage, gotCause, tt.wantOutage, tt.wantCause)
			}
		})
	}
}

func up(sec int) Sample   { return sample(sec, true, true, true, true) }
func down(sec int) Sample { return sample(sec, true, true, false, false) }

func newTracker() *Tracker {
	return NewTracker(TrackerConfig{FailThreshold: 3, MergeGap: 10 * time.Second})
}

func TestTrackerConfirmsOutageAfterThreshold(t *testing.T) {
	tr := newTracker()

	st, o := tr.Observe(down(0))
	if st.State != StateDegraded || o != nil {
		t.Fatalf("1st failure = (%v, %v), want degraded without outage", st.State, o)
	}
	st, o = tr.Observe(down(2))
	if st.State != StateDegraded || o != nil {
		t.Fatalf("2nd failure = (%v, %v), want degraded without outage", st.State, o)
	}
	st, o = tr.Observe(down(4))
	if st.State != StateDown || st.Cause != CauseLine {
		t.Fatalf("3rd failure status = %+v, want down by line", st)
	}
	if o == nil || !o.Open() || !o.Start.Equal(at(0)) || o.Cause != CauseLine {
		t.Fatalf("3rd failure outage = %+v, want open since first failure", o)
	}
}

func TestTrackerClosesShortOutageWithMeasuredDuration(t *testing.T) {
	tr := newTracker()
	for _, s := range []Sample{down(0), down(2), down(4), down(6)} {
		tr.Observe(s)
	}

	st, o := tr.Observe(up(8))
	if st.State != StateUp {
		t.Fatalf("recovery state = %v, want up", st.State)
	}
	if o == nil || o.Open() {
		t.Fatalf("recovery outage = %+v, want closed", o)
	}
	if got := o.Duration(); got != 8*time.Second {
		t.Fatalf("Duration() = %v, want 8s", got)
	}
}

func TestTrackerIgnoresFailuresBelowThreshold(t *testing.T) {
	tr := newTracker()
	tr.Observe(down(0))
	tr.Observe(down(2))

	st, o := tr.Observe(up(4))
	if o != nil || st.State != StateUp {
		t.Fatalf("flap = (%+v, %v), want up and no outage", st, o)
	}
	// the streak restarts: two more failures are still below the threshold
	tr.Observe(down(6))
	if _, o := tr.Observe(down(8)); o != nil {
		t.Fatalf("outage = %+v, want none", o)
	}
}

func TestTrackerKeepsClosestCauseSeenDuringOutage(t *testing.T) {
	tr := newTracker()
	tr.Observe(down(0))
	tr.Observe(down(2))
	tr.Observe(down(4)) // line

	_, o := tr.Observe(sample(6, true, false, false, false)) // router now fails
	if o == nil || o.Cause != CauseRouter {
		t.Fatalf("cause = %+v, want router (closer than line)", o)
	}
	_, o = tr.Observe(down(8)) // back to line; the closest one stays
	if o == nil || o.Cause != CauseRouter {
		t.Fatalf("cause = %+v, want router to stay", o)
	}
}

func TestTrackerMergesOutagesCloserThanGap(t *testing.T) {
	tr := newTracker()
	for _, s := range []Sample{down(0), down(2), down(4)} {
		tr.Observe(s)
	}
	_, closed := tr.Observe(up(6))
	if closed == nil || closed.Open() {
		t.Fatalf("first outage = %+v, want closed", closed)
	}

	// next failure starts 4s after the first outage ended
	tr.Observe(down(10))
	tr.Observe(down(12))
	_, reopened := tr.Observe(down(14))
	if reopened == nil || !reopened.Open() {
		t.Fatalf("merged outage = %+v, want open", reopened)
	}
	if reopened.ID != closed.ID || !reopened.Start.Equal(at(0)) {
		t.Fatalf("merged outage = %+v, want the first one (ID %d)", reopened, closed.ID)
	}
}

func TestTrackerKeepsOutagesApartBeyondGap(t *testing.T) {
	tr := newTracker()
	for _, s := range []Sample{down(0), down(2), down(4)} {
		tr.Observe(s)
	}
	_, first := tr.Observe(up(6))

	// next failure starts 24s after the first outage ended
	tr.Observe(down(30))
	tr.Observe(down(32))
	_, second := tr.Observe(down(34))
	if second == nil || second.ID == first.ID || !second.Start.Equal(at(30)) {
		t.Fatalf("second outage = %+v, want a new outage from 30s", second)
	}
}

func TestTrackerDegradedWhenOnlyDNSFails(t *testing.T) {
	tr := newTracker()
	st, o := tr.Observe(sample(0, true, true, true, false))
	if st.State != StateDegraded || o != nil {
		t.Fatalf("dns-only failure = (%+v, %v), want degraded without outage", st, o)
	}
}
