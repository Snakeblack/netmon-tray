package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"time"
)

// ErrNotRunning means no engine answers on the socket.
var ErrNotRunning = errors.New("netmon-tray no está en marcha")

// ErrStopped means the engine closed a watch, usually because it exited;
// ctl treats it as ErrNotRunning.
var ErrStopped = errors.New("el motor se ha detenido")

const (
	replyTimeout  = 5 * time.Second
	exportTimeout = 30 * time.Second
)

// Client talks to the engine listening on one socket path.
type Client struct {
	Path string
}

// Do sends one request and returns the engine's response.
func (c Client) Do(req Request) (Response, error) {
	timeout := replyTimeout
	if req.Cmd == CmdExport {
		timeout = exportTimeout
	}
	conn, r, err := c.send(req, timeout)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	return readResponse(r)
}

// Watch sends watch and copies every event line to out, as the engine
// wrote it, until the engine closes the connection (ErrStopped) or a
// write to out fails. A rejected watch returns its *Error.
func (c Client) Watch(out io.Writer) error {
	conn, r, err := c.send(Request{V: Version, Cmd: CmdWatch}, replyTimeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	resp, err := readResponse(r)
	if err != nil {
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	conn.SetReadDeadline(time.Time{})
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && line[len(line)-1] == '\n' {
			if _, werr := out.Write(line); werr != nil {
				return werr
			}
		}
		if err != nil {
			return ErrStopped
		}
	}
}

func (c Client) send(req Request, timeout time.Duration) (net.Conn, *bufio.Reader, error) {
	conn, err := net.DialTimeout("unix", c.Path, replyTimeout)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) || isRefused(err) || missing(c.Path) {
			return nil, nil, fmt.Errorf("%w (sin socket en %s)", ErrNotRunning, c.Path)
		}
		return nil, nil, err
	}
	line, err := json.Marshal(req)
	if err == nil {
		conn.SetDeadline(time.Now().Add(timeout))
		_, err = conn.Write(append(line, '\n'))
	}
	if err != nil {
		conn.Close()
		return nil, nil, err
	}
	return conn, bufio.NewReader(conn), nil
}

// missing reports whether nothing is at path; Windows may report a missing
// socket as another dial error.
func missing(path string) bool {
	_, err := os.Lstat(path)
	return errors.Is(err, fs.ErrNotExist)
}

func readResponse(r *bufio.Reader) (Response, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return Response{}, fmt.Errorf("sin respuesta del motor: %w", err)
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return Response{}, fmt.Errorf("respuesta del motor no válida: %w", err)
	}
	return resp, nil
}
