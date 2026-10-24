// Package runkey starts the engine with the Windows session through the Run
// key of the current user ("netmon-tray service install|remove"). A Windows
// service would run outside the session, where no tray icon can show.
package runkey

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ValueName is the value the engine registers under the Run key.
const ValueName = "netmon-tray"

// RunKey is the key, under HKEY_CURRENT_USER, that Windows reads at sign-in.
const RunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// GUIName is the windowsgui build of the engine, which starts without a
// console window; it must sit next to netmon-tray.exe.
const GUIName = "netmon-trayw.exe"

// Exit codes of netmon-tray service.
const (
	ExitOK     = 0
	ExitFailed = 1
	ExitUsage  = 2
)

const usage = `Uso: netmon-tray service install|remove

  install   registra ` + GUIName + ` en HKCU\` + RunKey + ` (` + ValueName + `)
            para que arranque con la sesión, y lo arranca si no hay un motor en marcha
  remove    quita ese registro; el motor en marcha sigue hasta que lo cierres con Salir
`

// Run reads and writes the values of the Run key.
type Run interface {
	// Get returns the value, or ok false when it does not exist.
	Get(name string) (value string, ok bool, err error)
	Set(name, value string) error
	// Delete removes the value; a missing one is no error.
	Delete(name string) error
}

// Installer holds what Service needs, so tests replace the registry and the
// engine.
type Installer struct {
	Run        Run
	Executable string // the running netmon-tray binary
	// Running reports whether an engine answers on the control socket.
	Running func() bool
	// Start launches the engine at path without waiting for it.
	Start func(path string) error
}

// Service runs "netmon-tray service" with args and returns the exit code.
func Service(args []string, in Installer, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "help" {
		fmt.Fprint(stdout, usage)
		return ExitOK
	}
	if len(args) != 1 || (args[0] != "install" && args[0] != "remove") {
		fmt.Fprintf(stderr, "netmon-tray service: orden no válida %q\n\n%s", strings.Join(args, " "), usage)
		return ExitUsage
	}
	if args[0] == "remove" {
		return in.remove(stdout, stderr)
	}
	return in.install(stdout, stderr)
}

// GUIPath is the windowsgui engine next to executable, or executable itself
// when it is that build.
func GUIPath(executable string) string {
	if strings.EqualFold(filepath.Base(executable), GUIName) {
		return executable
	}
	return filepath.Join(filepath.Dir(executable), GUIName)
}

func (in Installer) install(stdout, stderr io.Writer) int {
	gui := GUIPath(in.Executable)
	if info, err := os.Stat(gui); err != nil || !info.Mode().IsRegular() {
		fmt.Fprintf(stderr, "netmon-tray service: falta %s junto a este binario (%s).\n"+
			"Viene en el zip de la release, o se compila con:\n"+
			"  go build -ldflags=-H=windowsgui -o %s ./cmd/netmon-tray\n", GUIName, gui, GUIName)
		return ExitFailed
	}
	// Run values are command lines: quote the path, which may hold spaces.
	want := `"` + gui + `"`
	old, ok, err := in.Run.Get(ValueName)
	if err != nil {
		fmt.Fprintf(stderr, "netmon-tray service: leer HKCU\\%s: %v\n", RunKey, err)
		return ExitFailed
	}
	if ok && old == want {
		fmt.Fprintf(stdout, "El arranque con la sesión ya está al día (HKCU\\%s\\%s)\n", RunKey, ValueName)
	} else {
		if err := in.Run.Set(ValueName, want); err != nil {
			fmt.Fprintf(stderr, "netmon-tray service: escribir HKCU\\%s: %v\n", RunKey, err)
			return ExitFailed
		}
		fmt.Fprintf(stdout, "%s arranca con la sesión (HKCU\\%s\\%s)\n", gui, RunKey, ValueName)
	}
	if in.Running() {
		fmt.Fprintln(stdout, "Ya hay un netmon-tray en marcha; el registrado arrancará con la próxima sesión")
		return ExitOK
	}
	if err := in.Start(gui); err != nil {
		fmt.Fprintf(stderr, "netmon-tray service: arrancar %s: %v\n", gui, err)
		return ExitFailed
	}
	fmt.Fprintln(stdout, "Motor en marcha")
	return ExitOK
}

func (in Installer) remove(stdout, stderr io.Writer) int {
	_, ok, err := in.Run.Get(ValueName)
	if err != nil {
		fmt.Fprintf(stderr, "netmon-tray service: leer HKCU\\%s: %v\n", RunKey, err)
		return ExitFailed
	}
	if !ok {
		fmt.Fprintf(stdout, "El arranque con la sesión no está registrado (HKCU\\%s\\%s)\n", RunKey, ValueName)
		return ExitOK
	}
	if err := in.Run.Delete(ValueName); err != nil {
		fmt.Fprintf(stderr, "netmon-tray service: borrar HKCU\\%s\\%s: %v\n", RunKey, ValueName, err)
		return ExitFailed
	}
	fmt.Fprintf(stdout, "Arranque con la sesión quitado de HKCU\\%s\n", RunKey)
	if in.Running() {
		fmt.Fprintln(stdout, "El motor en marcha sigue hasta que lo cierres con Salir")
	}
	return ExitOK
}
