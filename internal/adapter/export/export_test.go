package export

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Snakeblack/netmon-tray/internal/domain"
)

var (
	madrid = time.FixedZone("CEST", 2*60*60)
	t0     = time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
)

func closed(startSec, endSec int, cause domain.Cause) domain.Outage {
	start := t0.Add(time.Duration(startSec) * time.Second)
	end := t0.Add(time.Duration(endSec) * time.Second)
	return domain.Outage{ID: start.UnixNano(), Start: start, End: end, LastSeen: end, Cause: cause}
}

func open(startSec, seenSec int, cause domain.Cause) domain.Outage {
	start := t0.Add(time.Duration(startSec) * time.Second)
	return domain.Outage{ID: start.UnixNano(), Start: start, LastSeen: t0.Add(time.Duration(seenSec) * time.Second), Cause: cause}
}

func TestCSVWritesHeaderAndOneRowPerOutageInLocalTime(t *testing.T) {
	var buf bytes.Buffer
	err := CSV(&buf, []domain.Outage{closed(0, 8, domain.CauseRouter), open(60, 66, domain.CauseLine)}, madrid)
	if err != nil {
		t.Fatal(err)
	}
	want := "inicio,fin,duracion_s,causa,abierto\n" +
		"2026-10-23T14:00:00+02:00,2026-10-23T14:00:08+02:00,8,router,false\n" +
		"2026-10-23T14:01:00+02:00,,6,line,true\n"
	if buf.String() != want {
		t.Fatalf("CSV() =\n%s\nwant\n%s", buf.String(), want)
	}
}

func TestCSVOfEmptyHistoryIsJustTheHeader(t *testing.T) {
	var buf bytes.Buffer
	if err := CSV(&buf, nil, madrid); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "inicio,fin,duracion_s,causa,abierto\n" {
		t.Fatalf("CSV(nil) = %q, want only the header", buf.String())
	}
}

func TestJSONUsesTheSameFieldsAndNullEndForAnOpenOutage(t *testing.T) {
	var buf bytes.Buffer
	err := JSON(&buf, []domain.Outage{closed(0, 8, domain.CauseRouter), open(60, 66, domain.CauseLine)}, madrid)
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("JSON() is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(got) != 2 {
		t.Fatalf("JSON() has %d outages, want 2", len(got))
	}
	first, second := got[0], got[1]
	if first["inicio"] != "2026-10-23T14:00:00+02:00" || first["fin"] != "2026-10-23T14:00:08+02:00" ||
		first["duracion_s"] != float64(8) || first["causa"] != "router" || first["abierto"] != false {
		t.Fatalf("first outage = %v", first)
	}
	if end, present := second["fin"]; !present || end != nil || second["abierto"] != true || second["duracion_s"] != float64(6) {
		t.Fatalf("open outage = %v, want fin null, abierto true, duracion_s 6", second)
	}
}

func TestJSONOfEmptyHistoryIsAnEmptyArray(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, nil, madrid); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(buf.String()) != "[]" {
		t.Fatalf("JSON(nil) = %q, want []", buf.String())
	}
}

func TestExporterWritesBothFilesPrivatelyInItsDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "exports")
	e := New(dir)
	e.now = func() time.Time { return time.Date(2026, 10, 23, 9, 30, 5, 0, time.Local) }

	paths, err := e.Write([]domain.Outage{closed(0, 8, domain.CauseLink)})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		filepath.Join(dir, "netmon-historial-20261023-093005.csv"),
		filepath.Join(dir, "netmon-historial-20261023-093005.json"),
	}
	if len(paths) != 2 || paths[0] != want[0] || paths[1] != want[1] {
		t.Fatalf("Write() paths = %v, want %v", paths, want)
	}
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s permissions = %o, want no group or world access", filepath.Base(p), perm)
		}
		if info.Size() == 0 {
			t.Errorf("%s is empty", filepath.Base(p))
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o700 {
		t.Errorf("directory permissions = %o, want 700", perm)
	}
}

func TestExporterLeavesNoFilesWhenItCannotWriteBoth(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "exports")
	e := New(dir)
	e.now = func() time.Time { return time.Date(2026, 10, 23, 9, 30, 5, 0, time.Local) }
	// a directory where the JSON file should go makes only the second file fail
	if err := os.MkdirAll(filepath.Join(dir, "netmon-historial-20261023-093005.json"), 0o700); err != nil {
		t.Fatal(err)
	}

	if _, err := e.Write([]domain.Outage{closed(0, 8, domain.CauseLink)}); err == nil {
		t.Fatal("Write() error = nil, want the JSON failure")
	}
	if _, err := os.Stat(filepath.Join(dir, "netmon-historial-20261023-093005.csv")); !os.IsNotExist(err) {
		t.Fatalf("csv left behind after a failed export (stat error = %v)", err)
	}
}
