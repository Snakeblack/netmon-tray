//go:build !windows

package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/adapter/logging"
	"github.com/Snakeblack/netmon-tray/internal/adapter/omarchy"
	"github.com/Snakeblack/netmon-tray/internal/adapter/systemd"
	"github.com/Snakeblack/netmon-tray/internal/adapter/tray"
	omarchyplugin "github.com/Snakeblack/netmon-tray/omarchy-plugin"
)

const usage = `Uso: netmon-tray                  arranca el motor y el icono
     netmon-tray ctl              controla el motor en marcha (netmon-tray ctl help)
     netmon-tray plugin install   instala el panel de Omarchy
     netmon-tray service install  arranca el motor con la sesión (systemd --user)
     netmon-tray version          muestra la versión
`

// engineLog logs to stderr, which systemd --user keeps in the journal.
func engineLog() (*slog.Logger, func()) {
	return logging.New(os.Stderr), func() {}
}

func plugin(args []string) int {
	dir, err := omarchy.DefaultDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "netmon-tray plugin: sin carpeta de plugins: %v\n", err)
		return omarchy.ExitFailed
	}
	in := omarchy.Installer{Files: omarchyplugin.Files, Dir: dir, LookPath: exec.LookPath, Shell: omarchy.ExecShell, Wait: 50 * time.Millisecond,
		TrayIcon: tray.Title}
	// Without shell.json the tray icon is left as it is.
	in.ShellConfig, _ = omarchy.DefaultShellConfig()
	return omarchy.Plugin(args, in, os.Stdout, os.Stderr)
}

func service(args []string) int {
	dir, err := systemd.DefaultDir()
	if err != nil {
		fmt.Fprintf(os.Stderr, "netmon-tray service: sin carpeta de unidades: %v\n", err)
		return systemd.ExitFailed
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "netmon-tray service: sin ruta del binario: %v\n", err)
		return systemd.ExitFailed
	}
	in := systemd.Installer{Dir: dir, Executable: exe, LookPath: exec.LookPath, Systemctl: systemd.ExecSystemctl, Running: engineRunning}
	return systemd.Service(args, in, os.Stdout, os.Stderr)
}

// historyPath is $XDG_DATA_HOME/netmon-tray/history.db, or under ~/.local/share.
func historyPath() (string, error) {
	dir := os.Getenv("XDG_DATA_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(dir, "netmon-tray", "history.db"), nil
}
