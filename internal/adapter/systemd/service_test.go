//go:build !windows

// The Omarchy panel and the systemd unit exist on Linux only.

package systemd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeSystemctl answers is-active with active and fails the calls whose
// verb is in fail. It records the calls.
type fakeSystemctl struct {
	active string
	fail   map[string]error
	calls  []string
}

func (f *fakeSystemctl) run(args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	if err := f.fail[args[0]]; err != nil {
		return "", err
	}
	if args[0] == "is-active" {
		if f.active == "active" {
			return "active", nil
		}
		return f.active, errors.New("exit status 3")
	}
	return "", nil
}

func onPath(names ...string) func(string) (string, error) {
	return func(file string) (string, error) {
		for _, n := range names {
			if n == file {
				return "/usr/bin/" + file, nil
			}
		}
		return "", errors.New("not found")
	}
}

func installer(dir string, sc *fakeSystemctl, running bool) Installer {
	return Installer{Dir: dir, Executable: "/home/u/.local/bin/netmon-tray", LookPath: onPath("systemctl"), Systemctl: sc.run,
		Running: func() bool { return running }}
}

func run(in Installer, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Service(args, in, &out, &errOut)
	return code, out.String(), errOut.String()
}

func assertCalls(t *testing.T, got []string, want ...string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("llamadas a systemctl:\n%s\nquiero:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestUnit(t *testing.T) {
	unit := Unit("/home/u/.local/bin/netmon-tray")
	for _, want := range []string{
		`ExecStart="/home/u/.local/bin/netmon-tray"`,
		"Restart=on-failure\n",
		"RestartPreventExitStatus=4\n",
		"PartOf=graphical-session.target\n",
		"ConditionEnvironment=WAYLAND_DISPLAY\n",
		"WantedBy=graphical-session.target\n",
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("la unidad no tiene %q:\n%s", want, unit)
		}
	}
}

// A path with spaces, quotes, specifiers or variables stays one literal word.
func TestUnitQuotesExecutable(t *testing.T) {
	unit := Unit(`/home/a b/100%/$HOME/"x"\y`)
	want := `ExecStart="/home/a b/100%%/$$HOME/\"x\"\\y"` + "\n"
	if !strings.Contains(unit, want) {
		t.Errorf("la unidad no tiene %q:\n%s", want, unit)
	}
}

func TestInstallFresh(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "systemd", "user")
	sc := &fakeSystemctl{active: "inactive"}
	code, out, errOut := run(installer(dir, sc, false), "install")
	if code != ExitOK || errOut != "" {
		t.Fatalf("código %d, stderr %q", code, errOut)
	}
	path := filepath.Join(dir, UnitName)
	got, err := os.ReadFile(path)
	if err != nil || string(got) != Unit("/home/u/.local/bin/netmon-tray") {
		t.Errorf("unidad = %q, %v", got, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o644 {
		t.Errorf("permisos = %v, %v; quiero 0644", info.Mode().Perm(), err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("la carpeta tiene %d ficheros; quiero solo la unidad", len(entries))
	}
	assertCalls(t, sc.calls, "daemon-reload", "enable "+UnitName, "is-active "+UnitName, "start "+UnitName)
	if !strings.Contains(out, "Unidad escrita en "+path) || !strings.Contains(out, "Motor en marcha") {
		t.Errorf("stdout = %q", out)
	}
}

// After updating the binary the unit is the same, but the engine restarts.
func TestInstallCurrentRestartsActive(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, UnitName), []byte(Unit("/home/u/.local/bin/netmon-tray")), 0o644)
	sc := &fakeSystemctl{active: "active"}
	code, out, errOut := run(installer(dir, sc, true), "install")
	if code != ExitOK || errOut != "" {
		t.Fatalf("código %d, stderr %q", code, errOut)
	}
	assertCalls(t, sc.calls, "enable "+UnitName, "is-active "+UnitName, "restart "+UnitName)
	if !strings.Contains(out, "ya está al día") || !strings.Contains(out, "Motor reiniciado") {
		t.Errorf("stdout = %q", out)
	}
}

func TestInstallReplacesDifferentUnit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, UnitName)
	os.WriteFile(path, []byte("[Service]\nExecStart=/old/netmon-tray\n"), 0o644)
	sc := &fakeSystemctl{active: "active"}
	if code, _, errOut := run(installer(dir, sc, true), "install"); code != ExitOK {
		t.Fatalf("código %d, stderr %q", code, errOut)
	}
	if got, _ := os.ReadFile(path); string(got) != Unit("/home/u/.local/bin/netmon-tray") {
		t.Errorf("unidad = %q", got)
	}
	assertCalls(t, sc.calls, "daemon-reload", "enable "+UnitName, "is-active "+UnitName, "restart "+UnitName)
}

// A loose engine would make the unit's engine exit at once: enable only.
func TestInstallWithLooseEngine(t *testing.T) {
	dir := t.TempDir()
	sc := &fakeSystemctl{active: "inactive"}
	code, _, errOut := run(installer(dir, sc, true), "install")
	if code != ExitFailed {
		t.Errorf("código %d; quiero %d", code, ExitFailed)
	}
	assertCalls(t, sc.calls, "daemon-reload", "enable "+UnitName, "is-active "+UnitName)
	if !strings.Contains(errOut, "fuera de la unidad") || !strings.Contains(errOut, "systemctl --user start "+UnitName) {
		t.Errorf("stderr = %q", errOut)
	}
}

func TestInstallSystemctlFails(t *testing.T) {
	dir := t.TempDir()
	sc := &fakeSystemctl{active: "inactive", fail: map[string]error{"enable": errors.New("exit status 1: Failed to connect to bus")}}
	code, _, errOut := run(installer(dir, sc, false), "install")
	if code != ExitFailed || !strings.Contains(errOut, "systemctl --user enable "+UnitName+": exit status 1: Failed to connect to bus") {
		t.Errorf("código %d, stderr %q", code, errOut)
	}
	assertCalls(t, sc.calls, "daemon-reload", "enable "+UnitName)
}

func TestWithoutSystemctl(t *testing.T) {
	dir := t.TempDir()
	sc := &fakeSystemctl{}
	in := installer(dir, sc, false)
	in.LookPath = onPath()
	for _, verb := range []string{"install", "remove"} {
		code, _, errOut := run(in, verb)
		if code != ExitFailed || !strings.Contains(errOut, "systemctl no está en el PATH") {
			t.Errorf("%s: código %d, stderr %q", verb, code, errOut)
		}
	}
	if len(sc.calls) > 0 {
		t.Errorf("llamadas = %v", sc.calls)
	}
	if entries, _ := os.ReadDir(dir); len(entries) > 0 {
		t.Errorf("escribió %d ficheros", len(entries))
	}
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, UnitName)
	os.WriteFile(path, []byte(Unit("/x")), 0o644)
	sc := &fakeSystemctl{}
	code, out, errOut := run(installer(dir, sc, true), "remove")
	if code != ExitOK || errOut != "" {
		t.Fatalf("código %d, stderr %q", code, errOut)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("la unidad sigue: %v", err)
	}
	assertCalls(t, sc.calls, "disable --now "+UnitName, "daemon-reload")
	if !strings.Contains(out, "borrada") {
		t.Errorf("stdout = %q", out)
	}
}

func TestRemoveNotInstalled(t *testing.T) {
	sc := &fakeSystemctl{}
	code, out, _ := run(installer(t.TempDir(), sc, false), "remove")
	if code != ExitOK || !strings.Contains(out, "no está instalada") || len(sc.calls) > 0 {
		t.Errorf("código %d, stdout %q, llamadas %v", code, out, sc.calls)
	}
}

// A failed disable leaves the unit file, so remove can be repeated.
func TestRemoveDisableFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, UnitName)
	os.WriteFile(path, []byte(Unit("/x")), 0o644)
	sc := &fakeSystemctl{fail: map[string]error{"disable": errors.New("exit status 1")}}
	if code, _, _ := run(installer(dir, sc, false), "remove"); code != ExitFailed {
		t.Errorf("código %d; quiero %d", code, ExitFailed)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("la unidad desapareció: %v", err)
	}
}

func TestUsage(t *testing.T) {
	in := installer(t.TempDir(), &fakeSystemctl{}, false)
	for _, args := range [][]string{nil, {"start"}, {"install", "x"}} {
		if code, _, errOut := run(in, args...); code != ExitUsage || !strings.Contains(errOut, "Uso: netmon-tray service") {
			t.Errorf("%v: código %d, stderr %q", args, code, errOut)
		}
	}
	if code, out, _ := run(in, "help"); code != ExitOK || !strings.Contains(out, "Uso: netmon-tray service") {
		t.Errorf("help: código %d, stdout %q", code, out)
	}
}
