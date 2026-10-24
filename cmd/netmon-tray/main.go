// Command netmon-tray monitors the network by layers and shows its state in
// the system tray. "netmon-tray ctl" controls the running engine,
// "netmon-tray plugin install" installs the Omarchy panel it embeds and
// "netmon-tray service install" makes it a systemd --user unit.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/adapter/control"
	"github.com/Snakeblack/netmon-tray/internal/adapter/export"
	"github.com/Snakeblack/netmon-tray/internal/adapter/logging"
	"github.com/Snakeblack/netmon-tray/internal/adapter/omarchy"
	"github.com/Snakeblack/netmon-tray/internal/adapter/probe"
	"github.com/Snakeblack/netmon-tray/internal/adapter/storage"
	"github.com/Snakeblack/netmon-tray/internal/adapter/systemd"
	"github.com/Snakeblack/netmon-tray/internal/adapter/tray"
	"github.com/Snakeblack/netmon-tray/internal/app"
	"github.com/Snakeblack/netmon-tray/internal/domain"
	omarchyplugin "github.com/Snakeblack/netmon-tray/omarchy-plugin"
)

const (
	probeInterval = 2 * time.Second
	failThreshold = 3
	mergeGap      = 10 * time.Second
	retention     = 60 * 24 * time.Hour
)

const usage = `Uso: netmon-tray                  arranca el motor y el icono
     netmon-tray ctl              controla el motor en marcha (netmon-tray ctl help)
     netmon-tray plugin install   instala el panel de Omarchy
     netmon-tray service install  arranca el motor con la sesión (systemd --user)
     netmon-tray version          muestra la versión
`

func main() {
	args := os.Args[1:]
	switch {
	case len(args) > 0 && args[0] == "ctl":
		os.Exit(control.Ctl(args[1:], os.Stdout, os.Stderr))
	case len(args) > 0 && args[0] == "plugin":
		os.Exit(plugin(args[1:]))
	case len(args) > 0 && args[0] == "service":
		os.Exit(service(args[1:]))
	case len(args) == 1 && (args[0] == "version" || args[0] == "--version"):
		fmt.Println("netmon-tray", version())
		os.Exit(0)
	case len(args) > 0:
		fmt.Fprintf(os.Stderr, "netmon-tray: argumento desconocido %q\n%s", args[0], usage)
		os.Exit(control.ExitUsage)
	}
	log := logging.New(os.Stderr)

	historyPath, err := historyPath()
	if err != nil {
		log.Error("resolve history path", "err", err)
		os.Exit(1)
	}
	// Open holds the history lock, so a second engine stops here without
	// touching the file (ADR 0003), and only the owner claims the socket.
	store, err := storage.Open(historyPath)
	if errors.Is(err, storage.ErrInUse) {
		log.Error("open history", "path", historyPath, "err", err)
		os.Exit(systemd.ExitAlreadyRunning)
	}
	if err != nil {
		log.Error("open history", "path", historyPath, "err", err)
		os.Exit(1)
	}
	defer store.Close()

	ctl := listenControl(log)
	if ctl != nil {
		defer ctl.Close()
	}

	prober, err := probe.New(probe.DefaultConfig())
	if err != nil {
		log.Error("start probes", "err", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	tracker := domain.NewTracker(domain.TrackerConfig{FailThreshold: failThreshold, MergeGap: mergeGap})
	exporter := export.New(filepath.Join(filepath.Dir(historyPath), "exports"))
	// The engine needs the tray as its sink and the tray needs the engine to
	// export, so the tray calls through a closure bound once the engine exists.
	var engine *app.Engine
	t := tray.New(func(ctx context.Context) ([]string, error) {
		paths, err := engine.Export(ctx)
		if err != nil {
			log.Error("export history", "err", err)
			return nil, err
		}
		log.Info("history exported", "files", paths)
		return paths, nil
	})
	sinks := app.Sinks{t}
	if ctl != nil {
		sinks = append(sinks, ctl)
	}
	engine = app.NewEngine(prober, store, sinks, tracker, probeInterval, log,
		app.WithRetention(retention), app.WithExporter(exporter))
	if ctl != nil {
		ctl.Serve(engine)
		log.Info("control socket ready", "path", ctl.Path())
	}

	log.Info("netmon-tray started", "version", version(), "history", historyPath, "interval", probeInterval, "retention", retention)
	t.Run(ctx, cancel, func(ctx context.Context) {
		if err := engine.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Error("engine stopped", "err", err)
			cancel()
		}
	})
	log.Info("netmon-tray stopped")
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

// engineRunning reports whether an engine answers on the control socket.
func engineRunning() bool {
	dir, err := control.DefaultDir()
	if err != nil {
		return false
	}
	conn, err := net.DialTimeout("unix", filepath.Join(dir, control.SocketName), time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// version is the module version Go stamps from the git tag: v0.1.0 on the
// tagged commit, a pseudo-version past it and "+dirty" with local changes.
func version() string {
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}

// listenControl opens the control socket. Without it the engine still runs
// with its tray icon; only another engine already running is fatal.
func listenControl(log *slog.Logger) *control.Server {
	dir, err := control.DefaultDir()
	if err != nil {
		log.Warn("control socket disabled", "err", err)
		return nil
	}
	srv, err := control.Listen(dir, log)
	if errors.Is(err, control.ErrAlreadyRunning) {
		log.Error("start control socket", "err", err)
		os.Exit(systemd.ExitAlreadyRunning)
	}
	if err != nil {
		log.Warn("control socket disabled", "err", err)
		return nil
	}
	return srv
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
