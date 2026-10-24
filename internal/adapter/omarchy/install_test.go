package omarchy

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

var plugin = fstest.MapFS{
	"manifest.json": {Data: []byte(`{"id":"snakeblack.netmon-tray"}`)},
	"Panel.qml":     {Data: []byte("Item {}\n")},
	"js/Model.js":   {Data: []byte("var x = 1\n")},
}

// fakeShell answers each IPC call from answers[method], one per call; the
// last answer repeats. It records the calls.
type fakeShell struct {
	answers map[string][]string
	fail    error
	calls   []string
	argv    [][]string
}

func (f *fakeShell) run(args ...string) (string, error) {
	f.calls = append(f.calls, strings.Join(args, " "))
	f.argv = append(f.argv, args)
	if f.fail != nil {
		return "", f.fail
	}
	queue := f.answers[args[1]]
	if len(queue) == 0 {
		return "", nil
	}
	answer := queue[0]
	if len(queue) > 1 {
		f.answers[args[1]] = queue[1:]
	}
	return answer, nil
}

func okShell() *fakeShell {
	return &fakeShell{answers: map[string][]string{"putBarWidget": {"ok"}, "setBarWidget": {"ok"}}}
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

func installer(dir string, files fs.FS, shell *fakeShell) Installer {
	return Installer{Files: files, Dir: dir, LookPath: onPath("omarchy-shell", "netmon-tray"), Shell: shell.run}
}

func run(in Installer, args ...string) (code int, stdout, stderr string) {
	var out, errOut bytes.Buffer
	code = Plugin(args, in, &out, &errOut)
	return code, out.String(), errOut.String()
}

func assertPlugin(t *testing.T, target string, files fstest.MapFS) {
	t.Helper()
	for name, f := range files {
		path := filepath.Join(target, filepath.FromSlash(name))
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, f.Data) {
			t.Errorf("%s = %q, %v; quiero %q", name, got, err, f.Data)
			continue
		}
		if info, _ := os.Stat(path); info.Mode().Perm() != 0o644 {
			t.Errorf("%s: permisos %v", name, info.Mode().Perm())
		}
	}
	for _, dir := range []string{target, filepath.Join(target, "js")} {
		if info, err := os.Stat(dir); err != nil || info.Mode().Perm() != 0o755 {
			t.Errorf("%s: %v, %v", dir, info, err)
		}
	}
}

func assertNoStage(t *testing.T, dir string) {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			t.Errorf("queda %s en la carpeta de plugins", e.Name())
		}
	}
}

func TestPluginUsage(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{}, {"remove"}, {"install", "--force"}} {
		code, out, errOut := run(installer(dir, plugin, okShell()), args...)
		if code != ExitUsage || out != "" || !strings.Contains(errOut, "Uso: netmon-tray plugin install") {
			t.Errorf("%v: code %d, stdout %q, stderr %q", args, code, out, errOut)
		}
	}
	code, out, _ := run(installer(dir, plugin, okShell()), "help")
	if code != ExitOK || !strings.Contains(out, "Uso: netmon-tray plugin install") {
		t.Errorf("help: code %d, stdout %q", code, out)
	}
}

func TestInstallCopiesAndPutsTheWidgetOnTheBar(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugins") // created by install
	shell := okShell()
	code, out, errOut := run(installer(dir, plugin, shell), "install")
	if code != ExitOK || errOut != "" {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	assertPlugin(t, filepath.Join(dir, PluginID), plugin)
	assertNoStage(t, dir)
	want := []string{"shell rescanPlugins", "shell putBarWidget snakeblack.netmon-tray {}"}
	if strings.Join(shell.calls, "|") != strings.Join(want, "|") {
		t.Errorf("llamadas %q", shell.calls)
	}
	if !strings.Contains(out, "Plugin copiado") || !strings.Contains(out, "está en la barra") || strings.Contains(out, "omarchy-restart-shell") {
		t.Errorf("stdout %q", out)
	}
}

func TestInstallAgainLeavesAnEqualCopyAlone(t *testing.T) {
	dir := t.TempDir()
	run(installer(dir, plugin, okShell()), "install")
	panel := filepath.Join(dir, PluginID, "Panel.qml")
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(panel, past, past); err != nil {
		t.Fatal(err)
	}
	shell := okShell()
	code, out, _ := run(installer(dir, plugin, shell), "install")
	if code != ExitOK || !strings.Contains(out, "ya está al día") || strings.Contains(out, "omarchy-restart-shell") {
		t.Errorf("code %d, stdout %q", code, out)
	}
	if info, _ := os.Stat(panel); !info.ModTime().Equal(past) {
		t.Errorf("Panel.qml se reescribió: %v", info.ModTime())
	}
	if len(shell.calls) != 2 {
		t.Errorf("llamadas %q", shell.calls)
	}
}

func TestInstallReplacesAnOlderCopy(t *testing.T) {
	dir := t.TempDir()
	older := fstest.MapFS{
		"manifest.json": {Data: []byte(`{"id":"snakeblack.netmon-tray","version":"0.0.1"}`)},
		"Panel.qml":     {Data: []byte("Item {}\n")},
		"Old.qml":       {Data: []byte("Item {}\n")},
	}
	run(installer(dir, older, okShell()), "install")
	code, out, _ := run(installer(dir, plugin, okShell()), "install")
	if code != ExitOK || !strings.Contains(out, "Plugin actualizado") || !strings.Contains(out, "omarchy-restart-shell") {
		t.Errorf("code %d, stdout %q", code, out)
	}
	target := filepath.Join(dir, PluginID)
	assertPlugin(t, target, plugin)
	if _, err := os.Stat(filepath.Join(target, "Old.qml")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Old.qml sigue ahí: %v", err)
	}
	assertNoStage(t, dir)
}

// A copy that lacks a file of the plugin is not the same copy.
func TestInstallCompletesACopyMissingAFile(t *testing.T) {
	dir := t.TempDir()
	run(installer(dir, plugin, okShell()), "install")
	if err := os.Remove(filepath.Join(dir, PluginID, "js", "Model.js")); err != nil {
		t.Fatal(err)
	}
	code, out, _ := run(installer(dir, plugin, okShell()), "install")
	if code != ExitOK || !strings.Contains(out, "Plugin actualizado") {
		t.Errorf("code %d, stdout %q", code, out)
	}
	assertPlugin(t, filepath.Join(dir, PluginID), plugin)
}

// A copy with a file the plugin no longer has is not the same copy either.
func TestInstallDropsAFileThePluginNoLongerHas(t *testing.T) {
	dir := t.TempDir()
	withOld := fstest.MapFS{"Old.qml": {Data: []byte("Item {}\n")}}
	for name, f := range plugin {
		withOld[name] = f
	}
	run(installer(dir, withOld, okShell()), "install")
	code, out, _ := run(installer(dir, plugin, okShell()), "install")
	if code != ExitOK || !strings.Contains(out, "Plugin actualizado") {
		t.Errorf("code %d, stdout %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(dir, PluginID, "Old.qml")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Old.qml sigue ahí: %v", err)
	}
}

func TestInstallKeepsTheDevelopmentLink(t *testing.T) {
	dir := t.TempDir()
	checkout := filepath.Join(t.TempDir(), "omarchy-plugin")
	if err := os.Mkdir(checkout, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, PluginID)
	if err := os.Symlink(checkout, target); err != nil {
		t.Fatal(err)
	}
	shell := okShell()
	code, out, _ := run(installer(dir, plugin, shell), "install")
	if code != ExitOK || !strings.Contains(out, "enlace de desarrollo a "+checkout) || !strings.Contains(out, "está en la barra") {
		t.Errorf("code %d, stdout %q", code, out)
	}
	if dest, err := os.Readlink(target); err != nil || dest != checkout {
		t.Errorf("enlace %q, %v", dest, err)
	}
	if entries, _ := os.ReadDir(checkout); len(entries) != 0 {
		t.Errorf("se escribió en el checkout: %v", entries)
	}
	if len(shell.calls) != 2 {
		t.Errorf("llamadas %q", shell.calls)
	}
}

func TestInstallRefusesAGitClone(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, PluginID)
	if err := os.MkdirAll(filepath.Join(target, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	shell := okShell()
	code, _, errOut := run(installer(dir, plugin, shell), "install")
	if code != ExitFailed || !strings.Contains(errOut, "omarchy plugin update "+PluginID) {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 1 {
		t.Errorf("se tocó el clon: %v", entries)
	}
	if len(shell.calls) != 0 {
		t.Errorf("llamadas %q", shell.calls)
	}
}

func TestInstallRefusesAFileInTheWay(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, PluginID)
	f, err := os.Create(target)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	code, _, errOut := run(installer(dir, plugin, okShell()), "install")
	if code != ExitFailed || !strings.Contains(errOut, "no es una carpeta") {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
}

func TestInstallWithoutOmarchy(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plugins")
	in := installer(dir, plugin, okShell())
	in.LookPath = onPath("netmon-tray")
	code, out, errOut := run(in, "install")
	if code != ExitFailed || out != "" || !strings.Contains(errOut, "omarchy-shell no está en el PATH") {
		t.Errorf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
	if _, err := os.Stat(dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("se creó la carpeta de plugins: %v", err)
	}
}

func TestInstallWithTheShellStoppedLeavesTheCopy(t *testing.T) {
	dir := t.TempDir()
	shell := &fakeShell{fail: errors.New("exit status 1: no running instances")}
	code, out, errOut := run(installer(dir, plugin, shell), "install")
	if code != ExitFailed || !strings.Contains(out, "Plugin copiado") {
		t.Errorf("code %d, stdout %q", code, out)
	}
	for _, want := range []string{"omarchy-shell no responde", "no running instances", "omarchy-shell shell rescanPlugins", "omarchy plugin enable " + PluginID} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr %q sin %q", errOut, want)
		}
	}
	assertPlugin(t, filepath.Join(dir, PluginID), plugin)
}

func TestInstallWaitsForTheScan(t *testing.T) {
	shell := &fakeShell{answers: map[string][]string{"putBarWidget": {"not ready", "unknown", "ok"}}}
	code, _, errOut := run(installer(t.TempDir(), plugin, shell), "install")
	if code != ExitOK || len(shell.calls) != 4 {
		t.Errorf("code %d, stderr %q, llamadas %q", code, errOut, shell.calls)
	}
}

func TestInstallReportsTheShellRefusal(t *testing.T) {
	for name, tc := range map[string]struct {
		answers []string
		calls   int // rescanPlugins and the putBarWidget calls
	}{
		"error":     {[]string{"plugin is not a bar widget"}, 2},
		"never set": {[]string{"not ready"}, 1 + readyTries},
	} {
		answers := tc.answers
		shell := &fakeShell{answers: map[string][]string{"putBarWidget": answers}}
		code, out, errOut := run(installer(t.TempDir(), plugin, shell), "install")
		if code != ExitFailed || !strings.Contains(errOut, "no pudo poner el widget en la barra: "+answers[0]) || strings.Contains(out, "está en la barra") {
			t.Errorf("%s: code %d, stdout %q, stderr %q", name, code, out, errOut)
		}
		if len(shell.calls) != tc.calls {
			t.Errorf("%s: %d llamadas", name, len(shell.calls))
		}
	}
}

func TestInstallWarnsWhenNetmonTrayIsNotOnThePath(t *testing.T) {
	in := installer(t.TempDir(), plugin, okShell())
	in.LookPath = onPath("omarchy-shell")
	code, _, errOut := run(in, "install")
	if code != ExitOK || !strings.Contains(errOut, "«Comando de netmon-tray»") {
		t.Errorf("code %d, stderr %q", code, errOut)
	}
}

// withShellConfig is an installer whose shell.json holds the bar layout of
// right (a JSON array), so install also hides the engine's tray icon.
func withShellConfig(t *testing.T, right string, shell *fakeShell) Installer {
	t.Helper()
	config := filepath.Join(t.TempDir(), "shell.json")
	if err := os.WriteFile(config, []byte(`{"bar":{"layout":{"left":[],"center":[],"right":`+right+`}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	in := installer(t.TempDir(), plugin, shell)
	in.ShellConfig, in.TrayIcon = config, "netmon-tray"
	return in
}

// settings returns the setBarWidget calls the shell received, as
// "id key value selector" with the value trimmed.
func settings(shell *fakeShell) []string {
	var out []string
	for _, args := range shell.argv {
		if len(args) == 6 && args[1] == "setBarWidget" {
			out = append(out, strings.Join([]string{args[2], args[3], strings.TrimSpace(args[4]), args[5]}, " "))
		}
	}
	return out
}

// Quickshell's IPC client splits an argument that starts with "[" into
// several: a JSON list must not reach it that way.
func TestSetBarWidgetValuesNeverStartWithABracket(t *testing.T) {
	shell := okShell()
	run(withShellConfig(t, `[{"id":"omarchy.tray","hidden":[]}]`, shell), "install")
	if len(settings(shell)) != 2 {
		t.Fatalf("setBarWidget %q", settings(shell))
	}
	for _, args := range shell.argv {
		if args[1] == "setBarWidget" && strings.HasPrefix(args[4], "[") {
			t.Errorf("valor %q empieza por [", args[4])
		}
	}
}

func TestInstallHidesTheTrayIconOnce(t *testing.T) {
	shell := okShell()
	in := withShellConfig(t, `[{"id":"omarchy.tray","pinned":[],"hidden":["Cachy-Update"]},{"id":"snakeblack.netmon-tray"}]`, shell)
	code, out, errOut := run(in, "install")
	if code != ExitOK || errOut != "" {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	want := []string{`omarchy.tray hidden ["Cachy-Update","netmon-tray"] {}`, `snakeblack.netmon-tray trayIconHidden true {}`}
	if got := settings(shell); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("setBarWidget %q, quiero %q", got, want)
	}
	if !strings.Contains(out, "queda oculto") || !strings.Contains(out, "Show") {
		t.Errorf("stdout %q", out)
	}
}

func TestInstallLeavesTheTrayIconAloneOnceMarked(t *testing.T) {
	// The user pressed Show after an earlier install hid it.
	shell := okShell()
	in := withShellConfig(t, `[{"id":"omarchy.tray","hidden":[]},{"id":"snakeblack.netmon-tray","trayIconHidden":true}]`, shell)
	code, out, errOut := run(in, "install")
	if code != ExitOK || errOut != "" || len(settings(shell)) != 0 || strings.Contains(out, "oculto") {
		t.Errorf("code %d, stdout %q, stderr %q, setBarWidget %q", code, out, errOut, settings(shell))
	}
}

func TestInstallMarksAnIconTheUserAlreadyHid(t *testing.T) {
	shell := okShell()
	in := withShellConfig(t, `[{"id":"omarchy.tray","hidden":["netmon-tray"]},{"id":"snakeblack.netmon-tray"}]`, shell)
	if code, _, errOut := run(in, "install"); code != ExitOK || errOut != "" {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	if got := settings(shell); len(got) != 1 || got[0] != `snakeblack.netmon-tray trayIconHidden true {}` {
		t.Errorf("setBarWidget %q, quiero solo la marca", got)
	}
}

func TestInstallUsesTheFirstTrayOnTheBar(t *testing.T) {
	shell := okShell()
	in := withShellConfig(t, `[{"id":"omarchy.tray","hidden":["b"]}]`, shell)
	// The left section comes first, as the shell searches it.
	os.WriteFile(in.ShellConfig, []byte(`{"bar":{"layout":{"left":[{"id":"omarchy.tray","hidden":["a"]}],"right":[{"id":"omarchy.tray","hidden":["b"]},"snakeblack.netmon-tray"]}}}`), 0o644)
	run(in, "install")
	if got := settings(shell); len(got) == 0 || got[0] != `omarchy.tray hidden ["a","netmon-tray"] {}` {
		t.Errorf("setBarWidget %q", got)
	}
}

func TestInstallWithoutATrayHidesNothing(t *testing.T) {
	shell := okShell()
	in := withShellConfig(t, `[{"id":"snakeblack.netmon-tray"}]`, shell)
	code, out, errOut := run(in, "install")
	if code != ExitOK || errOut != "" || len(settings(shell)) != 0 || strings.Contains(out, "oculto") {
		t.Errorf("code %d, stdout %q, stderr %q, setBarWidget %q", code, out, errOut, settings(shell))
	}
}

func TestInstallWarnsWhenTheTrayIconCannotBeHidden(t *testing.T) {
	refused := &fakeShell{answers: map[string][]string{"putBarWidget": {"ok"}, "setBarWidget": {"widget entry must be an object"}}}
	unreadable := okShell()
	tests := map[string]struct {
		in    Installer
		shell *fakeShell
		want  string
	}{
		"refused":    {withShellConfig(t, `["omarchy.tray",{"id":"snakeblack.netmon-tray"}]`, refused), refused, "widget entry must be an object"},
		"unreadable": {withShellConfig(t, `[{"id":`, unreadable), unreadable, "shell.json"},
	}
	for name, tt := range tests {
		code, out, errOut := run(tt.in, "install")
		if code != ExitOK || !strings.Contains(out, "está en la barra") {
			t.Errorf("%s: code %d, stdout %q", name, code, out)
		}
		for _, want := range []string{"Aviso: no se pudo ocultar el icono antiguo", tt.want, "Hide"} {
			if !strings.Contains(errOut, want) {
				t.Errorf("%s: stderr %q sin %q", name, errOut, want)
			}
		}
		if name == "refused" && len(settings(tt.shell)) != 1 {
			t.Errorf("%s: tras el rechazo no debe apuntar la marca: %q", name, settings(tt.shell))
		}
	}
}
