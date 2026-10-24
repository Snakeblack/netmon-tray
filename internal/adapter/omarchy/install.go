// Package omarchy installs the Omarchy shell plugin that netmon-tray embeds:
// it copies the files to the shell's plugin folder, asks the running shell to
// scan them, puts the widget on the bar and, once, hides the engine's own
// tray icon, which the panel replaces ("netmon-tray plugin install").
package omarchy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PluginID is the id in the plugin's manifest and the name of its folder.
const PluginID = "snakeblack.netmon-tray"

// Exit codes of netmon-tray plugin.
const (
	ExitOK     = 0
	ExitFailed = 1
	ExitUsage  = 2
)

// readyTries bounds the putBarWidget calls while the shell is still scanning.
const readyTries = 40

const (
	// trayWidget is the bar widget of Omarchy's system tray.
	trayWidget = "omarchy.tray"
	// hiddenMark, in the widget's entry, records that install hid the tray
	// icon once, so a later install respects the user's Show.
	hiddenMark = "trayIconHidden"
)

const usage = `Uso: netmon-tray plugin install

  install   copia el plugin de Omarchy a ~/.config/omarchy/plugins/` + PluginID + `
            y pone el widget Cortes de red en la barra
`

// Shell runs one omarchy-shell IPC call and returns its trimmed answer.
type Shell func(args ...string) (string, error)

// Installer holds what Plugin needs, so tests replace the shell and the PATH.
type Installer struct {
	Files    fs.FS  // the plugin files, at the root
	Dir      string // the shell's plugin folder
	LookPath func(file string) (string, error)
	Shell    Shell
	Wait     time.Duration // between "not ready" answers of the shell
	// ShellConfig is the shell's shell.json and TrayIcon the id of the
	// engine's tray icon; either empty skips hiding it.
	ShellConfig string
	TrayIcon    string
}

type placement int

const (
	installed placement = iota
	updated
	current
	linked
)

// DefaultDir is the folder where omarchy-shell looks for third-party plugins.
// Omarchy fixes it under $HOME, without XDG_CONFIG_HOME.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "omarchy", "plugins"), nil
}

// DefaultShellConfig is the shell.json omarchy-shell keeps its bar in.
func DefaultShellConfig() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "omarchy", "shell.json"), nil
}

// ExecShell runs omarchy-shell from the PATH.
func ExecShell(args ...string) (string, error) {
	out, err := exec.Command("omarchy-shell", args...).Output()
	var exit *exec.ExitError
	if errors.As(err, &exit) && len(exit.Stderr) > 0 {
		err = fmt.Errorf("%w: %s", err, bytes.TrimSpace(exit.Stderr))
	}
	return strings.TrimSpace(string(out)), err
}

// Plugin runs "netmon-tray plugin" with args and returns the exit code.
func Plugin(args []string, in Installer, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "help" {
		fmt.Fprint(stdout, usage)
		return ExitOK
	}
	if len(args) != 1 || args[0] != "install" {
		fmt.Fprintf(stderr, "netmon-tray plugin: orden no válida %q\n\n%s", strings.Join(args, " "), usage)
		return ExitUsage
	}
	if _, err := in.LookPath("omarchy-shell"); err != nil {
		fmt.Fprintln(stderr, "netmon-tray plugin: omarchy-shell no está en el PATH; el plugin necesita Omarchy 4")
		return ExitFailed
	}
	target := filepath.Join(in.Dir, PluginID)
	place, err := in.place(target)
	if err != nil {
		fmt.Fprintf(stderr, "netmon-tray plugin: %v\n", err)
		return ExitFailed
	}
	switch place {
	case installed:
		fmt.Fprintf(stdout, "Plugin copiado en %s\n", target)
	case updated:
		fmt.Fprintf(stdout, "Plugin actualizado en %s\n", target)
	case current:
		fmt.Fprintf(stdout, "El plugin de %s ya está al día\n", target)
	case linked:
		dest, _ := os.Readlink(target)
		fmt.Fprintf(stdout, "%s es un enlace de desarrollo a %s; no se toca\n", target, dest)
	}
	if err := in.enable(); err != nil {
		fmt.Fprintf(stderr, "netmon-tray plugin: %v\nCon el shell en marcha, actívalo con:\n  omarchy-shell shell rescanPlugins\n  omarchy plugin enable %s\n", err, PluginID)
		return ExitFailed
	}
	fmt.Fprintln(stdout, "El widget Cortes de red está en la barra")
	switch hidden, err := in.hideTrayIcon(); {
	case err != nil:
		fmt.Fprintf(stderr, "Aviso: no se pudo ocultar el icono antiguo de la bandeja: %v\nPara ocultarlo: clic derecho en la flecha de la bandeja de Omarchy y Hide en %s\n", err, in.TrayIcon)
	case hidden:
		fmt.Fprintf(stdout, "El icono antiguo de la bandeja (%s) queda oculto; para verlo: clic derecho en la flecha de la bandeja y Show\n", in.TrayIcon)
	}
	if place == updated {
		fmt.Fprintln(stdout, "omarchy-restart-shell carga la versión nueva del panel")
	}
	if _, err := in.LookPath("netmon-tray"); err != nil {
		fmt.Fprintln(stderr, "Aviso: netmon-tray no está en el PATH; pon su ruta absoluta en el ajuste «Comando de netmon-tray» del widget")
	}
	return ExitOK
}

// place leaves the embedded files at target: a symbolic link is a
// development checkout and stays, a git clone belongs to omarchy plugin
// update, and a copy is replaced only when it differs.
func (in Installer) place(target string) (placement, error) {
	info, err := os.Lstat(target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return installed, in.write(target, false)
	case err != nil:
		return 0, err
	case info.Mode()&fs.ModeSymlink != 0:
		return linked, nil
	case !info.IsDir():
		return 0, fmt.Errorf("%s existe y no es una carpeta", target)
	}
	if _, err := os.Lstat(filepath.Join(target, ".git")); err == nil {
		return 0, fmt.Errorf("%s es un clon git; actualízalo con: omarchy plugin update %s", target, PluginID)
	}
	same, err := in.same(target)
	if err != nil {
		return 0, err
	}
	if same {
		return current, nil
	}
	return updated, in.write(target, true)
}

// same reports whether target holds exactly the embedded files.
func (in Installer) same(target string) (bool, error) {
	count := 0
	err := fs.WalkDir(in.Files, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		count++
		want, err := fs.ReadFile(in.Files, name)
		if err != nil {
			return err
		}
		got, err := os.ReadFile(filepath.Join(target, filepath.FromSlash(name)))
		if errors.Is(err, fs.ErrNotExist) || (err == nil && !bytes.Equal(got, want)) {
			return errDiffers
		}
		return err
	})
	if errors.Is(err, errDiffers) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	found := 0
	err = filepath.WalkDir(target, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			found++
		}
		return err
	})
	return found == count, err
}

var errDiffers = errors.New("differs")

// write copies the files to a hidden folder next to target, which the shell
// does not scan, and moves it into place, so the shell never sees half a
// plugin. An old copy is moved aside first and back if the move fails.
func (in Installer) write(target string, replace bool) error {
	if err := os.MkdirAll(in.Dir, 0o755); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(in.Dir, ".netmon-tray-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	if err := os.Chmod(stage, 0o755); err != nil {
		return err
	}
	err = fs.WalkDir(in.Files, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil || name == "." {
			return err
		}
		path := filepath.Join(stage, filepath.FromSlash(name))
		if d.IsDir() {
			return os.Mkdir(path, 0o755)
		}
		data, err := fs.ReadFile(in.Files, name)
		if err != nil {
			return err
		}
		return os.WriteFile(path, data, 0o644)
	})
	if err != nil {
		return err
	}
	if !replace {
		return os.Rename(stage, target)
	}
	old := stage + ".old"
	if err := os.Rename(target, old); err != nil {
		return err
	}
	if err := os.Rename(stage, target); err != nil {
		return errors.Join(err, os.Rename(old, target))
	}
	return os.RemoveAll(old)
}

// hideTrayIcon adds the engine's icon to the hidden icons of the first tray
// on the bar, the one setBarWidget finds, and marks the widget's entry. It
// does it once: with the mark, or without a tray, it changes nothing.
// hidden reports whether the icon was newly hidden.
func (in Installer) hideTrayIcon() (hidden bool, err error) {
	if in.ShellConfig == "" || in.TrayIcon == "" {
		return false, nil
	}
	data, err := os.ReadFile(in.ShellConfig)
	if err != nil {
		return false, err
	}
	var config struct {
		Bar struct {
			Layout map[string][]json.RawMessage `json:"layout"`
		} `json:"bar"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return false, fmt.Errorf("%s no se puede leer: %v", in.ShellConfig, err)
	}
	var tray map[string]any
	trayFound := false
	for _, section := range []string{"left", "center", "right"} {
		for _, raw := range config.Bar.Layout[section] {
			var entry map[string]any
			if json.Unmarshal(raw, &entry) != nil {
				// An entry may be just the id; setBarWidget refuses it.
				var id string
				json.Unmarshal(raw, &id)
				entry = map[string]any{"id": id}
			}
			switch entry["id"] {
			case PluginID:
				if entry[hiddenMark] == true {
					return false, nil
				}
			case trayWidget:
				if !trayFound {
					tray, trayFound = entry, true
				}
			}
		}
	}
	if !trayFound {
		return false, nil
	}
	list, _ := tray["hidden"].([]any)
	already := false
	for _, id := range list {
		if id == in.TrayIcon {
			already = true
		}
	}
	if !already {
		value, err := json.Marshal(append(list, in.TrayIcon))
		if err != nil {
			return false, err
		}
		if err := in.setBarWidget(trayWidget, "hidden", string(value)); err != nil {
			return false, err
		}
	}
	return !already, in.setBarWidget(PluginID, hiddenMark, "true")
}

// setBarWidget sets key of the widget id to the JSON value. The IPC client
// of Quickshell splits an argument that starts with "[" into several, so
// the value goes after a space, which JSON.parse skips.
func (in Installer) setBarWidget(id, key, value string) error {
	answer, err := in.Shell("shell", "setBarWidget", id, key, " "+value, "{}")
	if err != nil {
		return fmt.Errorf("omarchy-shell no responde: %w", err)
	}
	if answer != "ok" {
		return fmt.Errorf("omarchy-shell no cambió %s: %s", id, answer)
	}
	return nil
}

// enable asks the shell to scan the plugin folder and to put the widget on
// the bar. putBarWidget leaves a widget that is already there where it is,
// and answers "not ready" until the scan, a subprocess, reaches the plugin.
func (in Installer) enable() error {
	if _, err := in.Shell("shell", "rescanPlugins"); err != nil {
		return fmt.Errorf("omarchy-shell no responde: %w", err)
	}
	for try := 1; ; try++ {
		answer, err := in.Shell("shell", "putBarWidget", PluginID, "{}")
		if err != nil {
			return fmt.Errorf("omarchy-shell no responde: %w", err)
		}
		if answer == "ok" {
			return nil
		}
		if (answer != "not ready" && answer != "unknown") || try == readyTries {
			return fmt.Errorf("omarchy-shell no pudo poner el widget en la barra: %s", answer)
		}
		time.Sleep(in.Wait)
	}
}
