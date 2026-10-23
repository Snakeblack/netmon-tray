package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/app"
	"github.com/Snakeblack/netmon-tray/internal/domain"
)

var t0 = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)

type fakeExporter struct {
	files []string
	err   error
}

func (f fakeExporter) Export(context.Context) ([]string, error) { return f.files, f.err }

// shortDir returns a directory whose socket path stays under the Unix limit.
func shortDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "nm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "netmon-tray")
}

func startServer(t *testing.T, x Exporter) *Server {
	t.Helper()
	srv, err := Listen(shortDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.Serve(x)
	t.Cleanup(func() { srv.Close() })
	return srv
}

func down(cause domain.Cause, last *domain.Outage) app.Status {
	return app.Status{Status: domain.Status{State: domain.StateDown, Cause: cause}, Last: last}
}

func openOutage(cause domain.Cause, seen time.Time) *domain.Outage {
	return &domain.Outage{ID: t0.UnixNano(), Start: t0, LastSeen: seen, Cause: cause}
}

// raw sends one line and returns the reader of the connection.
func raw(t *testing.T, path, line string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
	return conn, bufio.NewReader(conn)
}

func readLine[T any](t *testing.T, r *bufio.Reader) T {
	t.Helper()
	line, err := r.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var v T
	if err := json.Unmarshal(line, &v); err != nil {
		t.Fatalf("decode %q: %v", line, err)
	}
	return v
}

func TestStatusReturnsPublishedState(t *testing.T) {
	srv := startServer(t, nil)
	srv.Publish(down(domain.CauseRouter, openOutage(domain.CauseRouter, t0.Add(7*time.Second))))

	resp, err := Client{Path: srv.Path()}.Do(Request{V: Version, Cmd: CmdStatus})
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.State == nil {
		t.Fatalf("response = %+v", resp)
	}
	st := resp.State
	if st.State != "down" || st.Cause != "router" {
		t.Fatalf("state = %s/%s, want down/router", st.State, st.Cause)
	}
	if st.Last == nil || !st.Last.Open || st.Last.End != nil || st.Last.DurationS != 7 {
		t.Fatalf("last = %+v, want open outage of 7 s without end", st.Last)
	}
}

func TestStatusBeforeAnySampleIsUnknown(t *testing.T) {
	srv := startServer(t, nil)
	resp, err := Client{Path: srv.Path()}.Do(Request{V: Version, Cmd: CmdStatus})
	if err != nil {
		t.Fatal(err)
	}
	if resp.State == nil || resp.State.State != "unknown" || resp.State.Last != nil {
		t.Fatalf("state = %+v, want unknown without last outage", resp.State)
	}
}

func TestClosedOutageCarriesItsEnd(t *testing.T) {
	srv := startServer(t, nil)
	closed := &domain.Outage{ID: t0.UnixNano(), Start: t0, End: t0.Add(12 * time.Second), LastSeen: t0.Add(12 * time.Second), Cause: domain.CauseLine}
	srv.Publish(app.Status{Status: domain.Status{State: domain.StateUp}, Last: closed})

	resp, err := Client{Path: srv.Path()}.Do(Request{V: Version, Cmd: CmdStatus})
	if err != nil {
		t.Fatal(err)
	}
	last := resp.State.Last
	if last == nil || last.Open || last.End == nil || !last.End.Equal(closed.End) || last.DurationS != 12 || last.Cause != "line" {
		t.Fatalf("last = %+v", last)
	}
}

func TestRejectsBadRequests(t *testing.T) {
	srv := startServer(t, nil)
	cases := []struct {
		name, line, code string
	}{
		{"not json", "hola", CodeBadRequest},
		{"future version", `{"v":2,"cmd":"status"}`, CodeUnsupportedVersion},
		{"missing version", `{"cmd":"status"}`, CodeUnsupportedVersion},
		{"unknown command", `{"v":1,"cmd":"reboot"}`, CodeUnknownCommand},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, r := raw(t, srv.Path(), tc.line)
			resp := readLine[Response](t, r)
			if resp.OK || resp.Error == nil || resp.Error.Code != tc.code {
				t.Fatalf("response = %+v, want error %s", resp, tc.code)
			}
		})
	}
}

func TestRejectsOversizedRequest(t *testing.T) {
	srv := startServer(t, nil)
	_, r := raw(t, srv.Path(), string(make([]byte, maxLine+10)))
	resp := readLine[Response](t, r)
	if resp.Error == nil || resp.Error.Code != CodeBadRequest {
		t.Fatalf("response = %+v, want bad_request", resp)
	}
}

func TestWatchSendsCurrentStateThenEachChange(t *testing.T) {
	srv := startServer(t, nil)
	srv.Publish(app.Status{Status: domain.Status{State: domain.StateUp}})

	_, r := raw(t, srv.Path(), `{"v":1,"cmd":"watch"}`)
	if resp := readLine[Response](t, r); !resp.OK {
		t.Fatalf("watch rejected: %+v", resp)
	}
	if ev := readLine[Event](t, r); ev.Event != EventState || ev.State.State != "up" {
		t.Fatalf("first event = %+v, want state up", ev)
	}

	out := openOutage(domain.CauseLine, t0)
	srv.Publish(down(domain.CauseLine, out))
	if ev := readLine[Event](t, r); ev.State.State != "down" || ev.State.Cause != "line" {
		t.Fatalf("event = %+v, want down/line", ev)
	}
}

func TestWatchIgnoresSamplesThatChangeNothing(t *testing.T) {
	srv := startServer(t, nil)
	srv.Publish(down(domain.CauseLine, openOutage(domain.CauseLine, t0)))

	_, r := raw(t, srv.Path(), `{"v":1,"cmd":"watch"}`)
	readLine[Response](t, r)
	readLine[Event](t, r)

	// The open outage grows on every sample; that is not a state change.
	srv.Publish(down(domain.CauseLine, openOutage(domain.CauseLine, t0.Add(2*time.Second))))
	srv.Publish(down(domain.CauseLine, openOutage(domain.CauseLine, t0.Add(4*time.Second))))
	// Closing it is one.
	closed := openOutage(domain.CauseLine, t0.Add(6*time.Second))
	closed.End = t0.Add(6 * time.Second)
	srv.Publish(app.Status{Status: domain.Status{State: domain.StateUp}, Last: closed})

	ev := readLine[Event](t, r)
	if ev.State.State != "up" {
		t.Fatalf("event = %+v, want the closing change and nothing before it", ev)
	}
}

func TestWatchEndsWhenServerCloses(t *testing.T) {
	srv, err := Listen(shortDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.Serve(nil)

	_, r := raw(t, srv.Path(), `{"v":1,"cmd":"watch"}`)
	readLine[Response](t, r)
	readLine[Event](t, r)
	srv.Close()
	if _, err := r.ReadBytes('\n'); err == nil {
		t.Fatal("watch stayed open after Close")
	}
}

func TestExport(t *testing.T) {
	files := []string{"/x/h.csv", "/x/h.json"}
	cases := []struct {
		name string
		x    Exporter
		code string
	}{
		{"writes the files", fakeExporter{files: files}, ""},
		{"fails", fakeExporter{err: errors.New("disk full")}, CodeExportFailed},
		{"not configured", nil, CodeExportUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := startServer(t, tc.x)
			resp, err := Client{Path: srv.Path()}.Do(Request{V: Version, Cmd: CmdExport})
			if err != nil {
				t.Fatal(err)
			}
			if tc.code != "" {
				if resp.OK || resp.Error == nil || resp.Error.Code != tc.code {
					t.Fatalf("response = %+v, want error %s", resp, tc.code)
				}
				if resp.Error.Message == "disk full" {
					t.Fatal("internal error leaked to the client")
				}
				return
			}
			if !resp.OK || len(resp.Files) != 2 || resp.Files[0] != files[0] {
				t.Fatalf("response = %+v, want files %v", resp, files)
			}
		})
	}
}

func TestSocketIsPrivate(t *testing.T) {
	srv := startServer(t, nil)
	sock, err := os.Stat(srv.Path())
	if err != nil {
		t.Fatal(err)
	}
	if sock.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode = %v, want 0600", sock.Mode().Perm())
	}
	dir, err := os.Stat(filepath.Dir(srv.Path()))
	if err != nil {
		t.Fatal(err)
	}
	if dir.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, want 0700", dir.Mode().Perm())
	}
}

func TestLooseDirectoryIsTightened(t *testing.T) {
	dir := shortDir(t)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	srv, err := Listen(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	info, _ := os.Stat(dir)
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, want 0700", info.Mode().Perm())
	}
}

// A connection from another user gets no answer: the engine closes it.
func TestRejectsOtherUsers(t *testing.T) {
	srv, err := Listen(shortDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.uid = os.Getuid() + 1 // every peer now looks foreign
	srv.Serve(nil)
	defer srv.Close()

	conn, err := net.Dial("unix", srv.Path())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	conn.Write([]byte(`{"v":1,"cmd":"status"}` + "\n"))
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err == nil {
		t.Fatalf("a foreign user got an answer: %q", line)
	}
}

func TestSecondEngineIsRefused(t *testing.T) {
	first := startServer(t, nil)
	_, err := Listen(filepath.Dir(first.Path()), nil)
	if !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("err = %v, want ErrAlreadyRunning", err)
	}
	// The first engine keeps serving.
	if _, err := (Client{Path: first.Path()}).Do(Request{V: Version, Cmd: CmdStatus}); err != nil {
		t.Fatalf("first engine broken by the refused one: %v", err)
	}
}

func TestOrphanSocketIsReplaced(t *testing.T) {
	dir := shortDir(t)
	old, err := Listen(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Kill the listener without letting it unlink: a crashed engine.
	old.ln.SetUnlinkOnClose(false)
	old.ln.Close()
	old.releaseLock()
	if _, err := os.Lstat(old.Path()); err != nil {
		t.Fatalf("leftover socket missing: %v", err)
	}

	srv, err := Listen(dir, nil)
	if err != nil {
		t.Fatalf("orphan socket was not replaced: %v", err)
	}
	srv.Serve(nil)
	defer srv.Close()
	if _, err := (Client{Path: srv.Path()}).Do(Request{V: Version, Cmd: CmdStatus}); err != nil {
		t.Fatal(err)
	}
}

func TestNonSocketOnTheSocketPathIsKept(t *testing.T) {
	dir := shortDir(t)
	path := filepath.Join(dir, SocketName)
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(dir, nil); err == nil {
		t.Fatal("Listen replaced something that is not a socket")
	}
	if info, err := os.Lstat(path); err != nil || !info.IsDir() {
		t.Fatalf("the entry on the socket path was removed or replaced: %v", err)
	}
}

func TestDefaultDir(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	dir, err := DefaultDir()
	if err != nil || dir != "/run/user/1000/netmon-tray" {
		t.Fatalf("DefaultDir() = %q, %v", dir, err)
	}
	for _, bad := range []string{"", "relative/dir"} {
		t.Setenv("XDG_RUNTIME_DIR", bad)
		if _, err := DefaultDir(); err == nil {
			t.Fatalf("DefaultDir() accepted XDG_RUNTIME_DIR=%q", bad)
		}
	}
}

// Two engines can both find the path free; the one that loses the bind must
// see the winner as running, not as a failed socket.
func TestAddressInUseMeansAlreadyRunning(t *testing.T) {
	orig := listenUnix
	t.Cleanup(func() { listenUnix = orig })
	listenUnix = func(network string, addr *net.UnixAddr) (*net.UnixListener, error) {
		return nil, &net.OpError{Op: "listen", Net: network, Err: os.NewSyscallError("bind", syscall.EADDRINUSE)}
	}
	if _, err := Listen(shortDir(t), nil); !errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("err = %v, want ErrAlreadyRunning", err)
	}
}

// Only a refused connection proves a socket is a leftover. Any other dial
// failure leaves it alone.
func TestUnreachableSocketIsNotRemoved(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores the socket mode")
	}
	dir := shortDir(t)
	old, err := Listen(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	old.ln.SetUnlinkOnClose(false)
	old.ln.Close()
	old.releaseLock()
	path := old.Path()
	if err := os.Chmod(path, 0); err != nil { // dial now fails with EACCES
		t.Fatal(err)
	}

	if _, err := Listen(dir, nil); err == nil || errors.Is(err, ErrAlreadyRunning) {
		t.Fatalf("err = %v, want a failure that is not ErrAlreadyRunning", err)
	}
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("the socket was removed on an ambiguous dial error: %v", err)
	}
}

// Engines with different data directories share no history lock; the
// control directory must still let exactly one of them own the socket.
func TestConcurrentListenHasOneWinner(t *testing.T) {
	dir := shortDir(t)
	leaveOrphan(t, dir) // every engine finds it and wants to replace it
	const engines = 8
	type result struct {
		srv *Server
		err error
	}
	results := make(chan result, engines)
	start := make(chan struct{})
	for i := 0; i < engines; i++ {
		go func() {
			<-start
			srv, err := Listen(dir, nil)
			results <- result{srv, err}
		}()
	}
	close(start)

	var winner *Server
	for i := 0; i < engines; i++ {
		r := <-results
		switch {
		case r.err == nil && winner == nil:
			winner = r.srv
		case r.err == nil:
			r.srv.Close()
			t.Fatal("two engines own the socket")
		case !errors.Is(r.err, ErrAlreadyRunning):
			t.Fatalf("loser err = %v, want ErrAlreadyRunning", r.err)
		}
	}
	if winner == nil {
		t.Fatal("no engine owns the socket")
	}
	winner.Serve(nil)
	defer winner.Close()
	if _, err := (Client{Path: winner.Path()}).Do(Request{V: Version, Cmd: CmdStatus}); err != nil {
		t.Fatalf("the winner's socket is not reachable: %v", err)
	}
}

// A persistent accept failure (EMFILE) must not spin the loop, and Close
// must still stop it at once.
func TestAcceptFailuresBackOff(t *testing.T) {
	srv, err := Listen(shortDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	srv.acceptFn = func() (*net.UnixConn, error) {
		calls.Add(1)
		return nil, os.NewSyscallError("accept", syscall.EMFILE)
	}
	srv.Serve(nil)
	time.Sleep(200 * time.Millisecond)
	if n := calls.Load(); n > 10 {
		t.Fatalf("accept retried %d times in 200 ms, want a backoff", n)
	}
	closed := make(chan struct{})
	go func() { srv.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("Close waited for the backoff")
	}
}

// leaveOrphan leaves the socket of an engine that crashed in dir.
func leaveOrphan(t *testing.T, dir string) string {
	t.Helper()
	old, err := Listen(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	old.ln.SetUnlinkOnClose(false)
	old.ln.Close()
	old.releaseLock() // the kernel drops a dead process's lock
	return old.Path()
}
