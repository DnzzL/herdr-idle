package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// claudeSessions reads Claude Code's session files: one JSON line per
// event, under <config dir>/projects/<project>/<session id>.jsonl.
type claudeSessions struct {
	getenv func(string) string
}

func (c claudeSessions) ClaudeEntrypoint(_ context.Context, id string) (string, error) {
	// The id lands in a glob: anything but a plain id matches nothing.
	if id == "" || strings.ContainsAny(id, `/\*?[]`) || strings.HasPrefix(id, ".") {
		return "", fmt.Errorf("claude session %q: %w", id, fs.ErrNotExist)
	}
	dir := c.getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home := c.getenv("HOME")
		if home == "" {
			return "", errors.New("cannot locate Claude's config directory: CLAUDE_CONFIG_DIR and HOME are unset")
		}
		dir = filepath.Join(home, ".claude")
	}
	paths, err := filepath.Glob(filepath.Join(dir, "projects", "*", id+".jsonl"))
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("claude session %s: %w", id, fs.ErrNotExist)
	}
	f, err := os.Open(paths[0])
	if err != nil {
		return "", err
	}
	defer f.Close()
	// Lines can run to megabytes: read them whole, not with a Scanner.
	r := bufio.NewReader(f)
	key := []byte(`"entrypoint":"`)
	for {
		line, err := r.ReadBytes('\n')
		if _, rest, ok := bytes.Cut(line, key); ok {
			value, _, _ := bytes.Cut(rest, []byte(`"`))
			return string(value), nil
		}
		if errors.Is(err, io.EOF) {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("read %s: %w", paths[0], err)
		}
	}
}
