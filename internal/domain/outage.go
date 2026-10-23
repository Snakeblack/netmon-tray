package domain

import "time"

// Outage is a confirmed loss of connectivity. A zero End means it is still
// open. ID is the start instant in Unix nanoseconds, so it is stable and unique.
type Outage struct {
	ID       int64
	Start    time.Time
	End      time.Time
	LastSeen time.Time
	Cause    Cause
}

// Open reports whether the outage has not ended.
func (o Outage) Open() bool { return o.End.IsZero() }

// Duration is the elapsed time of the outage, up to LastSeen while it is open.
func (o Outage) Duration() time.Duration {
	if o.Open() {
		return o.LastSeen.Sub(o.Start)
	}
	return o.End.Sub(o.Start)
}

// TrackerConfig sets when failures become an outage.
type TrackerConfig struct {
	FailThreshold int           // consecutive failing samples that confirm an outage
	MergeGap      time.Duration // outages closer than this are one outage
}

// Status is what the tracker concludes after each sample.
type Status struct {
	State State
	Cause Cause
}

// Tracker turns a stream of samples into outages. It is not safe for
// concurrent use.
type Tracker struct {
	cfg TrackerConfig

	failStreak  int
	streakStart time.Time
	streakCause Cause

	current    *Outage
	lastClosed *Outage
}

// NewTracker builds a tracker; a threshold below 1 is treated as 1.
func NewTracker(cfg TrackerConfig) *Tracker {
	if cfg.FailThreshold < 1 {
		cfg.FailThreshold = 1
	}
	return &Tracker{cfg: cfg}
}

// Observe feeds one sample. It returns the new status and, when the outage
// changed (opened, still open, or closed), the outage to persist.
func (t *Tracker) Observe(s Sample) (Status, *Outage) {
	isOutage, cause := Classify(s)

	if isOutage {
		return t.fail(s, cause)
	}
	t.failStreak = 0
	if t.current != nil {
		closed := t.close(s.At)
		return t.statusFor(s), &closed
	}
	return t.statusFor(s), nil
}

func (t *Tracker) fail(s Sample, cause Cause) (Status, *Outage) {
	if t.failStreak == 0 {
		t.streakStart = s.At
		t.streakCause = cause
	}
	t.failStreak++
	t.streakCause = closest(t.streakCause, cause)

	if t.current != nil {
		t.current.Cause = closest(t.current.Cause, cause)
		t.current.LastSeen = s.At
		o := *t.current
		return Status{State: StateDown, Cause: o.Cause}, &o
	}
	if t.failStreak < t.cfg.FailThreshold {
		return Status{State: StateDegraded, Cause: t.streakCause}, nil
	}

	o := t.open(s.At)
	return Status{State: StateDown, Cause: o.Cause}, &o
}

// open starts an outage at the first failing sample, or reopens the previous
// one when it ended within MergeGap of that sample.
func (t *Tracker) open(at time.Time) Outage {
	var o Outage
	if prev := t.lastClosed; prev != nil && t.streakStart.Sub(prev.End) <= t.cfg.MergeGap {
		o = *prev
		o.End = time.Time{}
		o.Cause = closest(o.Cause, t.streakCause)
	} else {
		o = Outage{ID: t.streakStart.UnixNano(), Start: t.streakStart, Cause: t.streakCause}
	}
	o.LastSeen = at
	t.current = &o
	t.lastClosed = nil
	return o
}

func (t *Tracker) close(at time.Time) Outage {
	o := *t.current
	o.End = at
	o.LastSeen = at
	t.current = nil
	t.lastClosed = &o
	return o
}

func (t *Tracker) statusFor(s Sample) Status {
	if healthy(s) {
		return Status{State: StateUp}
	}
	return Status{State: StateDegraded}
}
