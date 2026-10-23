package control

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/app"
	"github.com/Snakeblack/netmon-tray/internal/domain"
)

func ctl(t *testing.T, path string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errb bytes.Buffer
	code = runCtl(args, path, &out, &errb)
	return code, out.String(), errb.String()
}

func TestCtlUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"reboot"}, {"status", "--bogus"}, {"status", "extra"}, {"watch", "x"}, {"export", "-x"}} {
		code, out, _ := ctl(t, "/nonexistent", args...)
		if code != ExitUsage || out != "" {
			t.Errorf("ctl %v = exit %d, stdout %q; want exit %d and no stdout", args, code, out, ExitUsage)
		}
	}
}

func TestCtlWithoutEngine(t *testing.T) {
	path := filepath.Join(shortDir(t), SocketName)
	for _, args := range [][]string{{"status"}, {"watch"}, {"export"}} {
		code, _, stderr := ctl(t, path, args...)
		if code != ExitNotRunning || !strings.Contains(stderr, "no está en marcha") {
			t.Errorf("ctl %v = exit %d, stderr %q; want exit %d", args, code, stderr, ExitNotRunning)
		}
	}
}

func TestCtlStatusText(t *testing.T) {
	srv := startServer(t, nil)
	outage := &domain.Outage{ID: t0.UnixNano(), Start: t0, End: t0.Add(12 * time.Second), LastSeen: t0.Add(12 * time.Second), Cause: domain.CauseRouter}
	srv.Publish(app.Status{Status: domain.Status{State: domain.StateUp}, Last: outage})

	code, stdout, stderr := ctl(t, srv.Path(), "status")
	if code != ExitOK || stderr != "" {
		t.Fatalf("exit %d, stderr %q", code, stderr)
	}
	for _, want := range []string{"Estado: Conectado", "Último corte:", "12s", "falla el router"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q misses %q", stdout, want)
		}
	}
}

func TestCtlStatusTextWhileDown(t *testing.T) {
	srv := startServer(t, nil)
	srv.Publish(down(domain.CauseLine, openOutage(domain.CauseLine, t0)))

	_, stdout, _ := ctl(t, srv.Path(), "status")
	for _, want := range []string{"Sin conexión: falla la línea u operador", "Corte en curso desde"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout %q misses %q", stdout, want)
		}
	}
}

func TestCtlStatusWithoutOutages(t *testing.T) {
	srv := startServer(t, nil)
	srv.Publish(app.Status{Status: domain.Status{State: domain.StateUp}})
	_, stdout, _ := ctl(t, srv.Path(), "status")
	if !strings.Contains(stdout, "Sin cortes registrados") {
		t.Errorf("stdout %q misses the empty history line", stdout)
	}
}

func TestCtlStatusJSON(t *testing.T) {
	srv := startServer(t, nil)
	srv.Publish(down(domain.CauseLink, openOutage(domain.CauseLink, t0)))

	code, stdout, _ := ctl(t, srv.Path(), "status", "--json")
	if code != ExitOK {
		t.Fatalf("exit %d", code)
	}
	var st State
	if err := json.Unmarshal([]byte(stdout), &st); err != nil {
		t.Fatalf("stdout is not JSON: %q", stdout)
	}
	if st.State != "down" || st.Cause != "link" || st.Last == nil {
		t.Fatalf("state = %+v", st)
	}
	if strings.Count(strings.TrimSpace(stdout), "\n") != 0 {
		t.Fatalf("status --json spans several lines: %q", stdout)
	}
}

func TestCtlExport(t *testing.T) {
	srv := startServer(t, fakeExporter{files: []string{"/x/h.csv", "/x/h.json"}})

	code, stdout, _ := ctl(t, srv.Path(), "export")
	if code != ExitOK || stdout != "/x/h.csv\n/x/h.json\n" {
		t.Fatalf("exit %d, stdout %q; want one path per line", code, stdout)
	}

	code, stdout, _ = ctl(t, srv.Path(), "export", "--json")
	var got struct{ Files []string }
	if err := json.Unmarshal([]byte(stdout), &got); code != ExitOK || err != nil || len(got.Files) != 2 {
		t.Fatalf("exit %d, stdout %q, err %v", code, stdout, err)
	}
}

func TestCtlExportRejected(t *testing.T) {
	srv := startServer(t, fakeExporter{err: errors.New("disk full")})
	code, stdout, stderr := ctl(t, srv.Path(), "export")
	if code != ExitRejected || stdout != "" || !strings.Contains(stderr, CodeExportFailed) {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestCtlWatchCopiesEventsAndReportsLostEngine(t *testing.T) {
	srv, err := Listen(shortDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	srv.Serve(nil)

	var out, errb syncBuffer
	done := make(chan int)
	go func() { done <- runCtl([]string{"watch"}, srv.Path(), &out, &errb) }()

	waitFor(t, func() bool { return strings.Count(out.String(), "\n") == 1 })
	srv.Publish(down(domain.CauseRouter, openOutage(domain.CauseRouter, t0)))
	waitFor(t, func() bool { return strings.Count(out.String(), "\n") == 2 })
	srv.Close()

	select {
	case code := <-done:
		if code != ExitNotRunning {
			t.Fatalf("exit %d, want %d when the engine goes away", code, ExitNotRunning)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch did not end with the engine")
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var ev Event
	if err := json.Unmarshal([]byte(lines[1]), &ev); err != nil || ev.State.State != "down" || ev.State.Cause != "router" {
		t.Fatalf("second line %q is not the down event", lines[1])
	}
}
