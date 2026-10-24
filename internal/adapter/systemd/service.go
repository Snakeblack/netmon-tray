// Package systemd installs the engine as a systemd --user unit that starts
// with the graphical session ("netmon-tray service install|remove").
package systemd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// UnitName is the unit's file name in the user unit folder.
const UnitName = "netmon-tray.service"

// ExitAlreadyRunning is the engine's exit code when another engine already
// holds the history or the control socket; the unit does not restart on it.
const ExitAlreadyRunning = 4

// Exit codes of netmon-tray service.
const (
	ExitOK     = 0
	ExitFailed = 1
	ExitUsage  = 2
)

const usage = `Uso: netmon-tray service install|remove

  install   instala el motor como unidad systemd --user (` + UnitName + `),
            la habilita con la sesión gráfica y la arranca, o la reinicia si ya estaba en marcha
  remove    para la unidad, la deshabilita y la borra
`

// unitTemplate takes the quoted ExecStart and ExitAlreadyRunning.
const unitTemplate = `[Unit]
Description=netmon-tray: monitor de cortes de red por capas
Documentation=https://github.com/Snakeblack/netmon-tray
# The tray icon and the panel need the compositor; follow the session.
After=graphical-session.target
PartOf=graphical-session.target
ConditionEnvironment=WAYLAND_DISPLAY

[Service]
Type=simple
ExecStart=%s
# Salir exits 0 and stays down; another engine already running is no failure.
Restart=on-failure
RestartSec=5
RestartPreventExitStatus=%d

[Install]
WantedBy=graphical-session.target
`

// Systemctl runs one "systemctl --user" call and returns its trimmed output.
type Systemctl func(args ...string) (string, error)

// Installer holds what Service needs, so tests replace systemctl and the
// engine probe.
type Installer struct {
	Dir        string // the user unit folder
	Executable string // the engine binary ExecStart runs
	LookPath   func(file string) (string, error)
	Systemctl  Systemctl
	// Running reports whether an engine answers on the control socket.
	Running func() bool
}

// DefaultDir is the user unit folder systemd reads,
// $XDG_CONFIG_HOME/systemd/user.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "systemd", "user"), nil
}

// ExecSystemctl runs systemctl --user from the PATH.
func ExecSystemctl(args ...string) (string, error) {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && len(exit.Stderr) > 0 {
		err = fmt.Errorf("%w: %s", err, bytes.TrimSpace(exit.Stderr))
	}
	return strings.TrimSpace(string(out)), err
}

// Unit is the unit file that runs executable.
func Unit(executable string) string {
	return fmt.Sprintf(unitTemplate, quote(executable), ExitAlreadyRunning)
}

// quote writes path as one ExecStart word: quoted, with the backslashes and
// quotes escaped and the specifiers (%) and variables ($) doubled.
func quote(path string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%", "$", "$$")
	return `"` + r.Replace(path) + `"`
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
	if _, err := in.LookPath("systemctl"); err != nil {
		fmt.Fprintln(stderr, "netmon-tray service: systemctl no está en el PATH; la unidad necesita systemd")
		return ExitFailed
	}
	if args[0] == "remove" {
		return in.remove(stdout, stderr)
	}
	return in.install(stdout, stderr)
}

func (in Installer) install(stdout, stderr io.Writer) int {
	path := filepath.Join(in.Dir, UnitName)
	changed, err := in.write(path, Unit(in.Executable))
	if err != nil {
		fmt.Fprintf(stderr, "netmon-tray service: %v\n", err)
		return ExitFailed
	}
	if changed {
		fmt.Fprintf(stdout, "Unidad escrita en %s\n", path)
		if err := in.run("daemon-reload"); err != nil {
			fmt.Fprintf(stderr, "netmon-tray service: %v\n", err)
			return ExitFailed
		}
	} else {
		fmt.Fprintf(stdout, "La unidad de %s ya está al día\n", path)
	}
	if err := in.run("enable", UnitName); err != nil {
		fmt.Fprintf(stderr, "netmon-tray service: %v\n", err)
		return ExitFailed
	}
	fmt.Fprintln(stdout, "La unidad arranca con la sesión gráfica")
	// is-active exits non-zero for anything but active; only the word counts.
	if state, _ := in.Systemctl("is-active", UnitName); state == "active" {
		if err := in.run("restart", UnitName); err != nil {
			fmt.Fprintf(stderr, "netmon-tray service: %v\n", err)
			return ExitFailed
		}
		fmt.Fprintln(stdout, "Motor reiniciado")
		return ExitOK
	}
	if in.Running() {
		fmt.Fprintf(stderr, "netmon-tray service: ya hay un netmon-tray en marcha fuera de la unidad; ciérralo con Salir y arranca la unidad con:\n  systemctl --user start %s\n", UnitName)
		return ExitFailed
	}
	if err := in.run("start", UnitName); err != nil {
		fmt.Fprintf(stderr, "netmon-tray service: %v\n", err)
		return ExitFailed
	}
	fmt.Fprintln(stdout, "Motor en marcha")
	return ExitOK
}

func (in Installer) remove(stdout, stderr io.Writer) int {
	path := filepath.Join(in.Dir, UnitName)
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		fmt.Fprintf(stdout, "La unidad no está instalada en %s\n", path)
		return ExitOK
	}
	if err := in.run("disable", "--now", UnitName); err != nil {
		fmt.Fprintf(stderr, "netmon-tray service: %v\n", err)
		return ExitFailed
	}
	if err := os.Remove(path); err != nil {
		fmt.Fprintf(stderr, "netmon-tray service: %v\n", err)
		return ExitFailed
	}
	if err := in.run("daemon-reload"); err != nil {
		fmt.Fprintf(stderr, "netmon-tray service: %v\n", err)
		return ExitFailed
	}
	fmt.Fprintf(stdout, "Unidad parada, deshabilitada y borrada de %s\n", path)
	return ExitOK
}

// write leaves unit at path, replacing it through a temporary file only
// when it differs; changed reports whether it wrote.
func (in Installer) write(path, unit string) (changed bool, err error) {
	old, err := os.ReadFile(path)
	if err == nil && string(old) == unit {
		return false, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(in.Dir, 0o755); err != nil {
		return false, err
	}
	tmp, err := os.CreateTemp(in.Dir, ".netmon-tray-*.tmp")
	if err != nil {
		return false, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(unit); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Chmod(0o644); err != nil {
		tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	return true, os.Rename(tmp.Name(), path)
}

func (in Installer) run(args ...string) error {
	if _, err := in.Systemctl(args...); err != nil {
		return fmt.Errorf("systemctl --user %s: %w", strings.Join(args, " "), err)
	}
	return nil
}
