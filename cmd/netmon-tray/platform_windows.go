package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Snakeblack/netmon-tray/internal/adapter/control"
	"github.com/Snakeblack/netmon-tray/internal/adapter/logging"
	"github.com/Snakeblack/netmon-tray/internal/adapter/runkey"
)

const usage = `Uso: netmon-tray                  arranca el motor y el icono
     netmon-tray ctl              controla el motor en marcha (netmon-tray ctl help)
     netmon-tray service install  arranca el motor con la sesión (` + runkey.GUIName + `)
     netmon-tray version          muestra la versión
`

// dataDir is %LOCALAPPDATA%\netmon-tray: history, exports and log.
func dataDir() (string, error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" || !filepath.IsAbs(base) {
		return "", errors.New("LOCALAPPDATA no está definido o no es una ruta absoluta")
	}
	return filepath.Join(base, "netmon-tray"), nil
}

// engineLog logs to netmon-tray.log in the data folder: Windows has no
// journal, and netmon-trayw.exe has no console. Without the file it falls
// back to stderr.
func engineLog() (*slog.Logger, func()) {
	dir, err := dataDir()
	if err != nil {
		return logging.New(os.Stderr), func() {}
	}
	f, err := logging.OpenFile(filepath.Join(dir, "netmon-tray.log"), logging.MaxFileSize)
	if err != nil {
		log := logging.New(os.Stderr)
		log.Warn("log file disabled", "err", err)
		return log, func() {}
	}
	return logging.New(f), func() { f.Close() }
}

// plugin explains that the Omarchy panel has no Windows counterpart.
func plugin([]string) int {
	fmt.Fprintln(os.Stderr, "netmon-tray plugin: no disponible en Windows; el estado está en el icono de la bandeja")
	return control.ExitUsage
}

func service(args []string) int {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "netmon-tray service: sin ruta del binario: %v\n", err)
		return runkey.ExitFailed
	}
	in := runkey.Installer{Run: runkey.Registry(), Executable: exe, Running: engineRunning, Start: startDetached}
	return runkey.Service(args, in, os.Stdout, os.Stderr)
}

// startDetached launches the engine and lets it outlive this command.
func startDetached(path string) error {
	cmd := exec.Command(path)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// historyPath is %LOCALAPPDATA%\netmon-tray\history.db.
func historyPath() (string, error) {
	dir, err := dataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.db"), nil
}
