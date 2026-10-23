// Package export writes the outage history as CSV and JSON files for the
// operator (ADR 0003).
package export

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/domain"
)

var csvHeader = []string{"inicio", "fin", "duracion_s", "causa", "abierto"}

// record is one outage as the operator reads it. Fin is null while the
// outage is open.
type record struct {
	Inicio    string  `json:"inicio"`
	Fin       *string `json:"fin"`
	DuracionS int64   `json:"duracion_s"`
	Causa     string  `json:"causa"`
	Abierto   bool    `json:"abierto"`
}

func toRecord(o domain.Outage, loc *time.Location) record {
	r := record{
		Inicio:    o.Start.In(loc).Format(time.RFC3339),
		DuracionS: int64(o.Duration().Round(time.Second) / time.Second),
		Causa:     string(o.Cause),
		Abierto:   o.Open(),
	}
	if !o.Open() {
		end := o.End.In(loc).Format(time.RFC3339)
		r.Fin = &end
	}
	return r
}

// CSV writes a header and one row per outage, with times in loc.
func CSV(w io.Writer, outages []domain.Outage, loc *time.Location) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}
	for _, o := range outages {
		r := toRecord(o, loc)
		end := ""
		if r.Fin != nil {
			end = *r.Fin
		}
		row := []string{r.Inicio, end, strconv.FormatInt(r.DuracionS, 10), r.Causa, strconv.FormatBool(r.Abierto)}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// JSON writes an array with one object per outage, with times in loc.
func JSON(w io.Writer, outages []domain.Outage, loc *time.Location) error {
	records := make([]record, 0, len(outages))
	for _, o := range outages {
		records = append(records, toRecord(o, loc))
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(records)
}

// Exporter writes the history files into a private directory.
type Exporter struct {
	dir string
	now func() time.Time
}

// New returns an exporter that writes into dir, creating it when needed.
func New(dir string) *Exporter {
	return &Exporter{dir: dir, now: time.Now}
}

// Write creates netmon-historial-<date>.csv and .json and returns their
// paths. When one file fails it removes the other, so an export is complete
// or absent.
func (e *Exporter) Write(outages []domain.Outage) ([]string, error) {
	if err := os.MkdirAll(e.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create export directory: %w", err)
	}
	// The history is private to the user, as is anything exported from it.
	if err := os.Chmod(e.dir, 0o700); err != nil {
		return nil, fmt.Errorf("restrict export directory: %w", err)
	}

	loc := time.Local
	stem := "netmon-historial-" + e.now().In(loc).Format("20060102-150405")
	files := []struct {
		ext   string
		write func(io.Writer, []domain.Outage, *time.Location) error
	}{{".csv", CSV}, {".json", JSON}}

	var paths []string
	for _, f := range files {
		path := filepath.Join(e.dir, stem+f.ext)
		if err := writeFile(path, outages, loc, f.write); err != nil {
			for _, written := range paths {
				os.Remove(written)
			}
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

func writeFile(path string, outages []domain.Outage, loc *time.Location, write func(io.Writer, []domain.Outage, *time.Location) error) (err error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	defer func() {
		if cerr := f.Close(); err == nil && cerr != nil {
			err = fmt.Errorf("write %s: %w", filepath.Base(path), cerr)
		}
		if err != nil {
			os.Remove(path)
		}
	}()
	if err := write(f, outages, loc); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}
