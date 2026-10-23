// Package control is the inbound adapter of ADR 0005: a local Unix socket
// that speaks the versioned JSON contract of docs/contract/control-v1.md,
// and the netmon-tray ctl client that uses it. The contract carries the
// state and the commands, never the history itself.
package control

import (
	"time"

	"github.com/Snakeblack/netmon-tray/internal/app"
	"github.com/Snakeblack/netmon-tray/internal/domain"
)

// Version is the contract version this engine speaks.
const Version = 1

// Commands of contract v1.
const (
	CmdStatus = "status"
	CmdWatch  = "watch"
	CmdExport = "export"
)

// EventState is the event watch pushes on every status change.
const EventState = "state"

// Error codes of contract v1.
const (
	CodeBadRequest         = "bad_request"
	CodeUnsupportedVersion = "unsupported_version"
	CodeUnknownCommand     = "unknown_command"
	CodeBusy               = "busy"
	CodeExportFailed       = "export_failed"
	CodeExportUnavailable  = "export_unavailable"
)

// Request is one line a client sends; each connection carries one.
type Request struct {
	V   int    `json:"v"`
	Cmd string `json:"cmd"`
}

// Response answers a request. State comes with status and Files with export;
// watch follows its response with events on the same connection.
type Response struct {
	V     int      `json:"v"`
	OK    bool     `json:"ok"`
	State *State   `json:"state,omitempty"`
	Files []string `json:"files,omitempty"`
	Error *Error   `json:"error,omitempty"`
}

// Error is a rejected request: Code for machines, Message for people.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Event is a line watch pushes after its response.
type Event struct {
	V     int    `json:"v"`
	Event string `json:"event"`
	State State  `json:"state"`
}

// State is the connection state plus the last known outage. State is one of
// unknown, up, degraded or down; Cause is link, router or line while down
// or degraded, and empty otherwise.
type State struct {
	State string  `json:"state"`
	Cause string  `json:"cause,omitempty"`
	Last  *Outage `json:"last,omitempty"`
}

// Outage is a confirmed loss of connectivity. End is absent while it is open.
type Outage struct {
	ID        int64      `json:"id"`
	Start     time.Time  `json:"start"`
	End       *time.Time `json:"end,omitempty"`
	LastSeen  time.Time  `json:"last_seen"`
	DurationS int64      `json:"duration_s"`
	Cause     string     `json:"cause"`
	Open      bool       `json:"open"`
}

// FromApp maps the engine status to its wire form.
func FromApp(s app.Status) State {
	st := State{State: s.State.String(), Cause: string(s.Cause)}
	if s.Last != nil {
		st.Last = fromOutage(*s.Last)
	}
	return st
}

func fromOutage(o domain.Outage) *Outage {
	out := &Outage{
		ID:        o.ID,
		Start:     o.Start,
		LastSeen:  o.LastSeen,
		DurationS: int64(o.Duration().Round(time.Second) / time.Second),
		Cause:     string(o.Cause),
		Open:      o.Open(),
	}
	if !o.Open() {
		end := o.End
		out.End = &end
	}
	return out
}

// Error implements error so a rejection can travel as one.
func (e *Error) Error() string {
	return e.Message
}
