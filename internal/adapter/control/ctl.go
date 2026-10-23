package control

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"time"
)

// Exit codes of ctl.
const (
	ExitOK         = 0 // the engine accepted the order
	ExitRejected   = 1 // the engine rejected it, or the exchange failed
	ExitUsage      = 2 // wrong arguments
	ExitNotRunning = 3 // no engine answers, also when a watch loses it
)

const usage = `Uso: netmon-tray ctl <orden>

Órdenes:
  status [--json]   Estado de la conexión y último corte
  watch             Un evento JSON por línea con cada cambio de estado
  export [--json]   Exporta el historial a CSV y JSON y muestra las rutas
`

// Ctl runs the ctl command against the engine of the current user and
// returns the process exit code.
func Ctl(args []string, stdout, stderr io.Writer) int {
	dir, err := DefaultDir()
	if err != nil {
		fmt.Fprintf(stderr, "%s (%v)\n", ErrNotRunning.Error(), err)
		return ExitNotRunning
	}
	return runCtl(args, filepath.Join(dir, SocketName), stdout, stderr)
}

func runCtl(args []string, path string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return ExitUsage
	}
	c := Client{Path: path}
	switch args[0] {
	case CmdStatus:
		return ctlStatus(c, args[1:], stdout, stderr)
	case CmdWatch:
		if len(args) > 1 {
			fmt.Fprint(stderr, usage)
			return ExitUsage
		}
		return ctlWatch(c, stdout, stderr)
	case CmdExport:
		return ctlExport(c, args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "Orden %q desconocida\n\n%s", args[0], usage)
		return ExitUsage
	}
}

// jsonFlag parses the only flag status and export take.
func jsonFlag(name string, args []string, stderr io.Writer) (asJSON, ok bool) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&asJSON, "json", false, "")
	if err := fs.Parse(args); err != nil || fs.NArg() > 0 {
		fmt.Fprint(stderr, usage)
		return false, false
	}
	return asJSON, true
}

func ctlStatus(c Client, args []string, stdout, stderr io.Writer) int {
	asJSON, ok := jsonFlag(CmdStatus, args, stderr)
	if !ok {
		return ExitUsage
	}
	resp, code := ask(c, Request{V: Version, Cmd: CmdStatus}, stderr)
	if code != ExitOK {
		return code
	}
	if resp.State == nil {
		fmt.Fprintln(stderr, "Respuesta del motor sin estado")
		return ExitRejected
	}
	if asJSON {
		return printJSON(stdout, stderr, resp.State)
	}
	fmt.Fprintf(stdout, "Estado: %s\n%s\n", stateText(*resp.State), lastText(resp.State.Last))
	return ExitOK
}

func ctlExport(c Client, args []string, stdout, stderr io.Writer) int {
	asJSON, ok := jsonFlag(CmdExport, args, stderr)
	if !ok {
		return ExitUsage
	}
	resp, code := ask(c, Request{V: Version, Cmd: CmdExport}, stderr)
	if code != ExitOK {
		return code
	}
	if asJSON {
		return printJSON(stdout, stderr, struct {
			Files []string `json:"files"`
		}{resp.Files})
	}
	for _, f := range resp.Files {
		fmt.Fprintln(stdout, f)
	}
	return ExitOK
}

func ctlWatch(c Client, stdout, stderr io.Writer) int {
	err := c.Watch(stdout)
	var rejected *Error
	switch {
	case errors.Is(err, ErrStopped), errors.Is(err, ErrNotRunning):
		fmt.Fprintln(stderr, ErrNotRunning.Error())
		return ExitNotRunning
	case errors.As(err, &rejected):
		fmt.Fprintf(stderr, "%s (%s)\n", rejected.Message, rejected.Code)
		return ExitRejected
	default:
		fmt.Fprintln(stderr, err)
		return ExitRejected
	}
}

// ask sends req and returns the accepted response, or the exit code that
// tells why there is none.
func ask(c Client, req Request, stderr io.Writer) (Response, int) {
	resp, err := c.Do(req)
	switch {
	case errors.Is(err, ErrNotRunning):
		fmt.Fprintln(stderr, err)
		return resp, ExitNotRunning
	case err != nil:
		fmt.Fprintln(stderr, err)
		return resp, ExitRejected
	case resp.Error != nil:
		fmt.Fprintf(stderr, "%s (%s)\n", resp.Error.Message, resp.Error.Code)
		return resp, ExitRejected
	}
	return resp, ExitOK
}

func printJSON(stdout, stderr io.Writer, v any) int {
	line, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ExitRejected
	}
	fmt.Fprintf(stdout, "%s\n", line)
	return ExitOK
}

func stateText(s State) string {
	switch s.State {
	case "up":
		return "Conectado"
	case "degraded":
		return "Conexión degradada"
	case "down":
		return "Sin conexión: " + causeText(s.Cause)
	default:
		return "Sin datos"
	}
}

func causeText(c string) string {
	switch c {
	case "link":
		return "falla el enlace local (wifi)"
	case "router":
		return "falla el router"
	case "line":
		return "falla la línea u operador"
	default:
		return "causa desconocida"
	}
}

func lastText(o *Outage) string {
	if o == nil {
		return "Sin cortes registrados"
	}
	start := o.Start.Local().Format("02/01 15:04:05")
	if o.Open {
		return fmt.Sprintf("Corte en curso desde %s", start)
	}
	return fmt.Sprintf("Último corte: %s, %s (%s)", start, time.Duration(o.DurationS)*time.Second, causeText(o.Cause))
}
