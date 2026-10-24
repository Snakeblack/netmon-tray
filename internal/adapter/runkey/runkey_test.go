package runkey

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRun is a Run key in memory; err fails every call.
type fakeRun struct {
	values map[string]string
	err    error
}

func (f *fakeRun) Get(name string) (string, bool, error) {
	if f.err != nil {
		return "", false, f.err
	}
	v, ok := f.values[name]
	return v, ok, nil
}

func (f *fakeRun) Set(name, value string) error {
	if f.err != nil {
		return f.err
	}
	f.values[name] = value
	return nil
}

func (f *fakeRun) Delete(name string) error {
	if f.err != nil {
		return f.err
	}
	delete(f.values, name)
	return nil
}

// engine records the engines Start launches.
type engine struct {
	running bool
	started []string
	err     error
}

func (e *engine) start(path string) error {
	if e.err != nil {
		return e.err
	}
	e.started = append(e.started, path)
	return nil
}

// release lays out netmon-tray.exe and, with gui, netmon-trayw.exe in a
// folder whose name has a space, as "Program Files" does.
func release(t *testing.T, gui bool) (exe, guiPath string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "net mon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	exe = filepath.Join(dir, "netmon-tray.exe")
	guiPath = filepath.Join(dir, GUIName)
	names := []string{exe}
	if gui {
		names = append(names, guiPath)
	}
	for _, p := range names {
		if err := os.WriteFile(p, []byte("MZ"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return exe, guiPath
}

func installer(exe string, run *fakeRun, e *engine) Installer {
	return Installer{Run: run, Executable: exe, Running: func() bool { return e.running }, Start: e.start}
}

func service(in Installer, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Service(args, in, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestInstallRegistersTheGUIEngineQuotedAndStartsIt(t *testing.T) {
	exe, gui := release(t, true)
	run, e := &fakeRun{values: map[string]string{}}, &engine{}
	code, stdout, stderr := service(installer(exe, run, e), "install")
	if code != ExitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := run.values[ValueName]; got != `"`+gui+`"` {
		t.Fatalf("Run value = %q, want the quoted path of %s", got, GUIName)
	}
	if len(e.started) != 1 || e.started[0] != gui {
		t.Fatalf("started = %v, want [%s]", e.started, gui)
	}
	if !strings.Contains(stdout, "Motor en marcha") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestInstallFromTheGUIBuildRegistersItself(t *testing.T) {
	_, gui := release(t, true)
	run, e := &fakeRun{values: map[string]string{}}, &engine{}
	if code, _, stderr := service(installer(gui, run, e), "install"); code != ExitOK {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if got := run.values[ValueName]; got != `"`+gui+`"` {
		t.Fatalf("Run value = %q", got)
	}
}

func TestInstallWithoutTheGUIBuildChangesNothing(t *testing.T) {
	exe, _ := release(t, false)
	run, e := &fakeRun{values: map[string]string{}}, &engine{}
	code, _, stderr := service(installer(exe, run, e), "install")
	if code != ExitFailed || !strings.Contains(stderr, GUIName) || !strings.Contains(stderr, "-H=windowsgui") {
		t.Fatalf("code = %d, stderr = %q, want a failure that says how to get %s", code, stderr, GUIName)
	}
	if len(run.values) != 0 || len(e.started) != 0 {
		t.Fatalf("values = %v, started = %v, want nothing", run.values, e.started)
	}
}

func TestInstallWithAnEngineRunningOnlyRegisters(t *testing.T) {
	exe, _ := release(t, true)
	run, e := &fakeRun{values: map[string]string{}}, &engine{running: true}
	code, stdout, _ := service(installer(exe, run, e), "install")
	if code != ExitOK || len(e.started) != 0 || run.values[ValueName] == "" {
		t.Fatalf("code = %d, started = %v, values = %v", code, e.started, run.values)
	}
	if !strings.Contains(stdout, "próxima sesión") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestInstallTwiceSaysItIsUpToDate(t *testing.T) {
	exe, gui := release(t, true)
	run, e := &fakeRun{values: map[string]string{ValueName: `"` + gui + `"`}}, &engine{running: true}
	code, stdout, _ := service(installer(exe, run, e), "install")
	if code != ExitOK || !strings.Contains(stdout, "ya está al día") {
		t.Fatalf("code = %d, stdout = %q", code, stdout)
	}
}

func TestInstallReplacesAnOldPath(t *testing.T) {
	exe, gui := release(t, true)
	run, e := &fakeRun{values: map[string]string{ValueName: `"C:\old\netmon-trayw.exe"`}}, &engine{running: true}
	if code, _, _ := service(installer(exe, run, e), "install"); code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	if got := run.values[ValueName]; got != `"`+gui+`"` {
		t.Fatalf("Run value = %q, want the new path", got)
	}
}

func TestInstallFailures(t *testing.T) {
	exe, _ := release(t, true)
	if code, _, stderr := service(installer(exe, &fakeRun{err: errors.New("access denied")}, &engine{}), "install"); code != ExitFailed || !strings.Contains(stderr, "access denied") {
		t.Errorf("registry failure: code = %d, stderr = %q", code, stderr)
	}
	run := &fakeRun{values: map[string]string{}}
	if code, _, stderr := service(installer(exe, run, &engine{err: errors.New("no such file")}), "install"); code != ExitFailed || !strings.Contains(stderr, "no such file") {
		t.Errorf("start failure: code = %d, stderr = %q", code, stderr)
	}
}

func TestRemoveDeletesTheValueAndLeavesTheEngine(t *testing.T) {
	exe, gui := release(t, true)
	run := &fakeRun{values: map[string]string{ValueName: `"` + gui + `"`, "Other": "x"}}
	code, stdout, _ := service(installer(exe, run, &engine{running: true}), "remove")
	if code != ExitOK {
		t.Fatalf("code = %d", code)
	}
	if _, ok := run.values[ValueName]; ok || run.values["Other"] != "x" {
		t.Fatalf("values = %v, want only %s removed", run.values, ValueName)
	}
	if !strings.Contains(stdout, "Salir") {
		t.Errorf("stdout = %q, want how to stop the running engine", stdout)
	}
}

func TestRemoveWithoutValueIsOK(t *testing.T) {
	exe, _ := release(t, true)
	code, stdout, _ := service(installer(exe, &fakeRun{values: map[string]string{}}, &engine{}), "remove")
	if code != ExitOK || !strings.Contains(stdout, "no está registrado") {
		t.Fatalf("code = %d, stdout = %q", code, stdout)
	}
}

func TestUsage(t *testing.T) {
	exe, _ := release(t, true)
	in := installer(exe, &fakeRun{values: map[string]string{}}, &engine{})
	if code, stdout, _ := service(in, "help"); code != ExitOK || !strings.Contains(stdout, "install") {
		t.Errorf("help: code = %d, stdout = %q", code, stdout)
	}
	for _, args := range [][]string{nil, {"start"}, {"install", "extra"}} {
		if code, _, stderr := service(in, args...); code != ExitUsage || !strings.Contains(stderr, "Uso:") {
			t.Errorf("%v: code = %d, stderr = %q", args, code, stderr)
		}
	}
}
