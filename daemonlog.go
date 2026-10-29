package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"github.com/DnzzL/herdr-idle/internal/herdr"
	"github.com/DnzzL/herdr-idle/internal/sweep"
)

// changeLog writes a pane's outcome only when it differs from the last one
// logged for that pane, so the daemon log tells what happened, not that
// nothing did. Errors are always written.
type changeLog struct {
	w    io.Writer
	now  func() time.Time
	last map[herdr.PaneID]string
}

func newChangeLog(w io.Writer, now func() time.Time) *changeLog {
	return &changeLog{w: w, now: now, last: map[herdr.PaneID]string{}}
}

func (l *changeLog) log(outcomes []sweep.Outcome, err error) {
	ts := l.now().Format(time.RFC3339)
	for _, o := range outcomes {
		// "quiet for 10m0s" and "quiet for 20m0s" are the same news.
		key := string(o.Agent) + "\t" + strings.TrimFunc(strings.Map(func(r rune) rune {
			if unicode.IsDigit(r) {
				return -1
			}
			return r
		}, o.Reason), unicode.IsSpace)
		if l.last[o.PaneID] == key {
			continue
		}
		l.last[o.PaneID] = key
		fmt.Fprintf(l.w, "%s\t%s\t%s\t%s\n", ts, o.PaneID, o.Agent, o.Reason)
	}
	if err != nil {
		fmt.Fprintf(l.w, "%s\terror: %v\n", ts, err)
	}
}

// rotatingFile appends to path and, before a write would take it past max
// bytes, moves it to path.1 (replacing the previous one): the log never
// holds more than about twice max.
type rotatingFile struct {
	path string
	max  int64
	f    *os.File
	size int64
}

func (r *rotatingFile) Write(p []byte) (int, error) {
	if r.f == nil {
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	if r.size > 0 && r.size+int64(len(p)) > r.max {
		if err := r.f.Close(); err != nil {
			return 0, err
		}
		r.f = nil
		if err := os.Rename(r.path, r.path+".1"); err != nil {
			return 0, err
		}
		if err := r.open(); err != nil {
			return 0, err
		}
	}
	n, err := r.f.Write(p)
	r.size += int64(n)
	return n, err
}

func (r *rotatingFile) open() error {
	f, err := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	r.f, r.size = f, st.Size()
	return nil
}

func (r *rotatingFile) Close() error {
	if r.f == nil {
		return nil
	}
	return r.f.Close()
}
