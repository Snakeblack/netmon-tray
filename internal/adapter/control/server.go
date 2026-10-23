package control

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/app"
)

// SocketName is the file the engine listens on inside the control directory.
const SocketName = "control.sock"

// lockName is the file whose flock makes one engine the owner of the socket.
const lockName = "control.lock"

// ErrAlreadyRunning means another engine answers on the socket.
var ErrAlreadyRunning = errors.New("ya hay un netmon-tray en marcha")

const (
	maxConns       = 16
	maxLine        = 4 << 10
	requestTimeout = 5 * time.Second
	writeTimeout   = 5 * time.Second
	probeTimeout   = time.Second
	minAcceptDelay = 5 * time.Millisecond
	maxAcceptDelay = time.Second
)

// listenUnix binds the socket; tests replace it to lose a bind race.
var listenUnix = net.ListenUnix

// Exporter runs the export command on behalf of a client.
type Exporter interface {
	Export(ctx context.Context) ([]string, error)
}

// DefaultDir is $XDG_RUNTIME_DIR/netmon-tray.
func DefaultDir() (string, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		return "", errors.New("XDG_RUNTIME_DIR no está definido")
	}
	if !filepath.IsAbs(base) {
		return "", fmt.Errorf("XDG_RUNTIME_DIR %q no es una ruta absoluta", base)
	}
	return filepath.Join(base, "netmon-tray"), nil
}

// Server accepts the clients of one engine. It implements app.Sink: each
// published status is kept for status and pushed to every watch.
type Server struct {
	ln       *net.UnixListener
	acceptFn func() (*net.UnixConn, error) // ln.AcceptUnix; tests make it fail
	lock     *os.File                      // held until Close has unlinked the socket
	exporter Exporter
	log      *slog.Logger
	uid      int
	slots    chan struct{}
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc

	mu     sync.Mutex // guards the fields below
	state  State
	subs   map[chan State]struct{}
	conns  map[net.Conn]struct{}
	closed bool
}

var _ app.Sink = (*Server)(nil)

// Listen creates dir with mode 0700 and the socket in it with mode 0600;
// Serve starts accepting. The engine that holds the flock of dir owns the
// socket; the others get ErrAlreadyRunning, as they do when an engine
// without the lock answers on it or binds it first. Under the lock a socket
// that refuses connections is a leftover and is replaced; any other failure
// to reach it leaves it in place and returns an error. log nil discards the
// log.
func Listen(dir string, log *slog.Logger) (srv *Server, err error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if err := privateDir(dir); err != nil {
		return nil, err
	}
	lock, err := lockDir(dir)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			lock.Close()
		}
	}()
	path := filepath.Join(dir, SocketName)
	if err := clearStale(path); err != nil {
		return nil, err
	}
	ln, err := listenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if errors.Is(err, syscall.EADDRINUSE) {
		// Another engine bound the path between clearStale and here.
		return nil, fmt.Errorf("%s: %w", path, ErrAlreadyRunning)
	}
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		ln.Close()
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		ln: ln, acceptFn: ln.AcceptUnix, lock: lock, log: log, uid: os.Getuid(),
		ctx: ctx, cancel: cancel,
		slots: make(chan struct{}, maxConns),
		state: FromApp(app.Status{}),
		subs:  map[chan State]struct{}{},
		conns: map[net.Conn]struct{}{},
	}, nil
}

// Serve accepts clients in the background; export runs on exporter. Call it
// once. A nil exporter makes export answer export_unavailable.
func (s *Server) Serve(exporter Exporter) {
	s.exporter = exporter
	s.wg.Add(1)
	go s.accept()
}

// privateDir makes dir a directory only its owner, the current user, can
// enter.
func privateDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s no es un directorio", dir)
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok && int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s pertenece a otro usuario", dir)
	}
	if info.Mode().Perm() != 0o700 {
		return os.Chmod(dir, 0o700)
	}
	return nil
}

func clearStale(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode().Type() != fs.ModeSocket {
		return fmt.Errorf("%s existe y no es un socket", path)
	}
	conn, err := net.DialTimeout("unix", path, probeTimeout)
	switch {
	case err == nil:
		conn.Close()
		return fmt.Errorf("%s: %w", path, ErrAlreadyRunning)
	case errors.Is(err, fs.ErrNotExist):
		return nil // removed since Lstat
	case errors.Is(err, syscall.ECONNREFUSED):
		return os.Remove(path)
	default:
		return fmt.Errorf("%s no responde y no se sabe si está huérfano: %w", path, err)
	}
}

// lockDir takes the owner lock of the control directory without waiting.
// The kernel drops it when the process exits.
func lockDir(dir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(dir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%s: %w", dir, ErrAlreadyRunning)
		}
		return nil, err
	}
	return f, nil
}

func (s *Server) releaseLock() {
	if s.lock != nil {
		s.lock.Close()
		s.lock = nil
	}
}

// Path is the socket's path.
func (s *Server) Path() string {
	return s.ln.Addr().String()
}

// Publish keeps the status for status and hands it to every watch without
// waiting: a watch that has not sent the previous state gets only the latest.
// It sends nothing when the status did not change.
func (s *Server) Publish(status app.Status) {
	state := FromApp(status)
	s.mu.Lock()
	defer s.mu.Unlock()
	if sameState(s.state, state) {
		return
	}
	s.state = state
	for ch := range s.subs {
		select {
		case <-ch:
		default:
		}
		ch <- state
	}
}

// sameState compares the fields a client sees. The last outage grows its
// duration on every sample, so it only counts when it opens, closes or
// changes cause.
func sameState(a, b State) bool {
	if a.State != b.State || a.Cause != b.Cause {
		return false
	}
	if (a.Last == nil) != (b.Last == nil) {
		return false
	}
	if a.Last == nil {
		return true
	}
	return a.Last.ID == b.Last.ID && a.Last.Open == b.Last.Open && a.Last.Cause == b.Last.Cause
}

// Close stops accepting, drops every client, removes the socket and waits
// for the handlers.
func (s *Server) Close() error {
	s.mu.Lock()
	s.closed = true
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	s.cancel()
	err := s.ln.Close() // also removes the socket file
	s.wg.Wait()
	s.releaseLock() // only now, so no other engine sees our socket as a leftover
	return err
}

func (s *Server) accept() {
	defer s.wg.Done()
	var delay time.Duration
	for {
		conn, err := s.acceptFn()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			// A lasting failure such as EMFILE would spin: wait, doubling
			// up to maxAcceptDelay, and log once per wait.
			delay = min(max(2*delay, minAcceptDelay), maxAcceptDelay)
			s.log.Warn("control: fallo al aceptar", "err", err, "retry_in", delay)
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(delay):
			}
			continue
		}
		delay = 0
		if uid, err := peerUID(conn); err != nil || uid != s.uid {
			s.log.Warn("control: conexión de otro usuario rechazada", "uid", uid, "err", err)
			conn.Close()
			continue
		}
		select {
		case s.slots <- struct{}{}:
		default:
			s.log.Warn("control: demasiadas conexiones")
			s.write(conn, reject(CodeBusy, "Demasiadas conexiones; inténtalo de nuevo"))
			conn.Close()
			continue
		}
		if !s.track(conn) {
			<-s.slots
			conn.Close()
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer func() { <-s.slots }()
			defer s.untrack(conn)
			s.serve(conn)
		}()
	}
}

func peerUID(conn *net.UnixConn) (int, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return -1, err
	}
	uid := -1
	var credErr error
	err = raw.Control(func(fd uintptr) {
		var cred *syscall.Ucred
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if credErr == nil {
			uid = int(cred.Uid)
		}
	})
	if err == nil {
		err = credErr
	}
	return uid, err
}

func (s *Server) track(c net.Conn) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.conns[c] = struct{}{}
	return true
}

func (s *Server) untrack(c net.Conn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
	c.Close()
}

// serve answers the one request of a connection.
func (s *Server) serve(conn net.Conn) {
	conn.SetReadDeadline(time.Now().Add(requestTimeout))
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 512), maxLine)
	if !scanner.Scan() {
		if err := scanner.Err(); errors.Is(err, bufio.ErrTooLong) {
			s.write(conn, reject(CodeBadRequest, fmt.Sprintf("Petición de más de %d bytes", maxLine)))
		}
		return
	}
	conn.SetReadDeadline(time.Time{})
	var req Request
	if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
		s.write(conn, reject(CodeBadRequest, "Petición no válida: no es un objeto JSON"))
		return
	}
	if req.V != Version {
		s.write(conn, reject(CodeUnsupportedVersion, fmt.Sprintf("Versión %d no admitida; este motor habla la %d", req.V, Version)))
		return
	}
	switch req.Cmd {
	case CmdWatch:
		s.watch(conn)
		return
	case CmdStatus:
		s.mu.Lock()
		state := s.state
		s.mu.Unlock()
		s.write(conn, Response{V: Version, OK: true, State: &state})
	case CmdExport:
		s.write(conn, s.export())
	default:
		s.log.Info("control: orden rechazada", "cmd", req.Cmd, "code", CodeUnknownCommand)
		s.write(conn, reject(CodeUnknownCommand, fmt.Sprintf("Orden %q desconocida", req.Cmd)))
	}
}

// export has the engine write the history and answers with the files.
func (s *Server) export() Response {
	if s.exporter == nil {
		return reject(CodeExportUnavailable, "La exportación no está disponible")
	}
	files, err := s.exporter.Export(s.ctx)
	if err != nil {
		s.log.Error("control: exportación fallida", "err", err)
		return reject(CodeExportFailed, "No se pudo exportar el historial")
	}
	return Response{V: Version, OK: true, Files: files}
}

// watch answers, sends the current state and then every new one until the
// client hangs up or the server closes.
func (s *Server) watch(conn net.Conn) {
	ch := make(chan State, 1)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	ch <- s.state
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()
	s.log.Debug("control: watch abierto")
	defer s.log.Debug("control: watch cerrado")

	if !s.write(conn, Response{V: Version, OK: true}) {
		return
	}
	// The client sends nothing more: a read returns when it hangs up.
	gone := make(chan struct{})
	go func() {
		defer close(gone)
		var b [1]byte
		for {
			if _, err := conn.Read(b[:]); err != nil {
				return
			}
		}
	}()
	for {
		select {
		case <-gone:
			return
		case state := <-ch:
			if !s.write(conn, Event{V: Version, Event: EventState, State: state}) {
				return
			}
		}
	}
}

// write sends v as one line; false means the client is gone.
func (s *Server) write(conn net.Conn, v any) bool {
	line, err := json.Marshal(v)
	if err != nil {
		s.log.Error("control: respuesta sin codificar", "err", err)
		return false
	}
	conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	if _, err := conn.Write(append(line, '\n')); err != nil {
		s.log.Debug("control: cliente perdido", "err", err)
		return false
	}
	return true
}

func reject(code, message string) Response {
	return Response{V: Version, Error: &Error{Code: code, Message: message}}
}
