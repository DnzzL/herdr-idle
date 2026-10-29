package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DnzzL/herdr-idle/internal/sweep"
)

func TestDaemonLogKeepsOnlyChanges(t *testing.T) {
	var b strings.Builder
	l := newChangeLog(&b, func() time.Time { return time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC) })
	quiet := func(d string) []sweep.Outcome {
		return []sweep.Outcome{
			{PaneID: "w1:p1", Agent: "claude", Reason: "quiet for " + d + ", needs 2h0m0s"},
			{PaneID: "w1:p2", Agent: "maki", Reason: "asleep"},
		}
	}

	l.log(quiet("10m0s"), nil)
	first := b.String()
	if strings.Count(first, "\n") != 2 {
		t.Fatalf("first sweep logged %q, want every pane once", first)
	}
	l.log(quiet("20m0s"), nil)
	if b.String() != first {
		t.Fatalf("unchanged sweep logged %q", strings.TrimPrefix(b.String(), first))
	}

	// A rearm check reports one pane only: the others are not forgotten.
	gone := []sweep.Outcome{{PaneID: "w1:p2", Agent: "claude", Reason: "pane not found, rearm on a later sweep"}}
	l.log(gone, nil)
	l.log(gone, nil)
	l.log(quiet("30m0s")[:1], nil)
	added := strings.TrimPrefix(b.String(), first)
	if added != "2026-10-29T12:00:00Z\tw1:p2\tclaude\tpane not found, rearm on a later sweep\n" {
		t.Fatalf("logged %q, want the vanished pane once", added)
	}

	l.log(nil, errors.New("herdr down"))
	l.log(nil, errors.New("herdr down"))
	if n := strings.Count(b.String(), "error: herdr down"); n != 2 {
		t.Fatalf("error logged %d times, want every time", n)
	}
}

func TestDaemonLogFileStaysBounded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.log")
	f := &rotatingFile{path: path, max: 100}
	defer f.Close()
	line := strings.Repeat("x", 59) + "\n"
	for range 10 {
		if _, err := f.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{path, path + ".1"} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if st.Size() > 100 {
			t.Fatalf("%s is %d bytes, want at most 100", p, st.Size())
		}
	}
	b, _ := os.ReadFile(path)
	if string(b) != line {
		t.Fatalf("current log = %q, want the last line", b)
	}
}
