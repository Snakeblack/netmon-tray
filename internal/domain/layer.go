// Package domain holds the network layers, outage classification and outage
// tracking rules. It performs no I/O and knows nothing about the OS.
package domain

import "time"

// Cause names the layer closest to the user that failed during an outage.
type Cause string

const (
	CauseNone   Cause = ""
	CauseLink   Cause = "link"   // local link (wifi or ethernet)
	CauseRouter Cause = "router" // default gateway unreachable
	CauseLine   Cause = "line"   // gateway answers but the internet does not: line or operator
)

// rank orders causes from the closest to the user to the farthest.
func (c Cause) rank() int {
	switch c {
	case CauseLink:
		return 1
	case CauseRouter:
		return 2
	case CauseLine:
		return 3
	default:
		return 4
	}
}

// closest returns the cause nearest to the user.
func closest(a, b Cause) Cause {
	if b.rank() < a.rank() {
		return b
	}
	return a
}

// State is the health of the connection as the user sees it.
type State int

const (
	StateUnknown  State = iota // no sample yet
	StateUp                    // every layer answers
	StateDegraded              // something fails, but no outage is confirmed
	StateDown                  // a confirmed outage is open
)

func (s State) String() string {
	switch s {
	case StateUp:
		return "up"
	case StateDegraded:
		return "degraded"
	case StateDown:
		return "down"
	default:
		return "unknown"
	}
}

// Sample is the result of probing every layer once.
type Sample struct {
	At       time.Time
	Link     bool
	Router   bool
	Internet bool
	DNS      bool
}
