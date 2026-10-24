package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func appendLine(t *testing.T, path, line string, max int64) {
	t.Helper()
	f, err := OpenFile(path, max)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(line); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestOpenFileCreatesItsDirectoryAndAppends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netmon-tray", "netmon-tray.log")
	appendLine(t, path, "a\n", 100)
	appendLine(t, path, "b\n", 100)
	if got := read(t, path); got != "a\nb\n" {
		t.Fatalf("log = %q, want both lines", got)
	}
	if _, err := os.Stat(path + ".old"); !os.IsNotExist(err) {
		t.Fatalf("a log under the limit was moved (stat error = %v)", err)
	}
}

func TestOpenFilePastTheLimitKeepsOnlyThePreviousLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netmon-tray.log")
	appendLine(t, path, "first run, long\n", 4)
	appendLine(t, path, "second run\n", 4) // the first log is past 4 bytes
	appendLine(t, path, "third run\n", 4)  // and so is the second

	if got := read(t, path); got != "third run\n" {
		t.Fatalf("log = %q, want only the third run", got)
	}
	if got := read(t, path+".old"); got != "second run\n" {
		t.Fatalf("old log = %q, want the second run, replacing the first", got)
	}
}

func TestOpenFileAppendsWhenTheLogCannotMove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "netmon-tray.log")
	appendLine(t, path, "first run, long\n", 4)
	// A non-empty directory where the old log goes makes the move fail.
	if err := os.MkdirAll(filepath.Join(path+".old", "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	appendLine(t, path, "second run\n", 4)
	if got := read(t, path); got != "first run, long\nsecond run\n" {
		t.Fatalf("log = %q, want the second run appended", got)
	}
}
