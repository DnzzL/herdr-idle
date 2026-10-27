// Package config reads herdr-idle's config.toml: four flat keys, written in
// the small subset of TOML they need.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Config is what the user can tune.
type Config struct {
	// IdleAfter is how long a pane must stay quiet before it is put asleep.
	IdleAfter time.Duration
	// Interval is how often the daemon sweeps.
	Interval time.Duration
	// Exclude holds pane ids and agent session ids never put asleep.
	Exclude []string
	// DryRun reports what a sweep would do and does nothing.
	DryRun bool
}

// Default is the config used when the file is missing, and for every key the
// file leaves out.
func Default() Config {
	return Config{IdleAfter: 60 * time.Minute, Interval: 10 * time.Minute}
}

// Excludes reports whether a pane or agent session id is excluded.
func (c Config) Excludes(id string) bool {
	return id != "" && slices.Contains(c.Exclude, id)
}

// Template is the file the config action creates: every key, commented out
// at its default.
const Template = `# herdr-idle config. Re-read on every sweep; a missing key uses its default.

# Minutes an agent pane must stay quiet before it is put asleep.
# idle_minutes = 60

# Minutes between two sweeps of the background daemon.
# interval_minutes = 10

# Pane ids (w1:p2) or agent session ids never put asleep, e.g. agents
# running a loop or a cron job.
# exclude = ["w1:p2", "be00b9d0-1111-2222-3333-444455556666"]

# Log what would be put asleep, and do nothing.
# dry_run = false
`

// File is a config file read again on every Load. It remembers the last valid
// config it read. Not safe for concurrent use.
type File struct {
	Path string
	last *Config
}

// Load reads the file. A missing file is the default config. When the file
// is invalid, err lists every problem and cfg is the last valid config this
// File loaded; ok reports whether there was one, so cfg can be used.
func (f *File) Load() (cfg Config, ok bool, err error) {
	b, err := os.ReadFile(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		b, err = nil, nil
	}
	if err == nil {
		cfg, err = Parse(string(b))
	}
	if err != nil {
		err = fmt.Errorf("config %s: %w", f.Path, err)
		if f.last == nil {
			return Config{}, false, err
		}
		return *f.last, true, err
	}
	f.last = &cfg
	return cfg, true, nil
}

// Parse reads a config file's text. Its error joins every problem found,
// each with its line number.
func Parse(text string) (Config, error) {
	cfg := Default()
	var errs []error
	seen := map[string]bool{}
	lines := strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		n := i + 1
		line := strings.TrimSpace(stripComment(lines[i]))
		if line == "" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !found || key == "" || value == "" {
			errs = append(errs, fmt.Errorf("line %d: want key = value, got %q", n, line))
			continue
		}
		// An array may span lines until its closing bracket.
		for strings.HasPrefix(value, "[") && !strings.HasSuffix(value, "]") && i+1 < len(lines) {
			i++
			value += " " + strings.TrimSpace(stripComment(lines[i]))
		}
		if seen[key] {
			errs = append(errs, fmt.Errorf("line %d: %s is set twice", n, key))
			continue
		}
		seen[key] = true
		var err error
		switch key {
		case "idle_minutes":
			cfg.IdleAfter, err = minutes(value)
		case "interval_minutes":
			cfg.Interval, err = minutes(value)
		case "exclude":
			cfg.Exclude, err = stringArray(value)
		case "dry_run":
			cfg.DryRun, err = boolean(value)
		default:
			err = errors.New("unknown key (known: idle_minutes, interval_minutes, exclude, dry_run)")
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("line %d: %s: %w", n, key, err))
		}
	}
	return cfg, errors.Join(errs...)
}

// stripComment drops a # comment, leaving any # inside a string alone.
func stripComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote != 0 && c == '\\' && quote == '"':
			i++
		case quote != 0 && c == quote:
			quote = 0
		case quote == 0 && (c == '"' || c == '\''):
			quote = c
		case quote == 0 && c == '#':
			return line[:i]
		}
	}
	return line
}

func minutes(v string) (time.Duration, error) {
	n, err := strconv.Atoi(strings.ReplaceAll(v, "_", ""))
	if err != nil {
		return 0, fmt.Errorf("want a whole number of minutes, got %s", v)
	}
	if n < 1 || n > 7*24*60 {
		return 0, fmt.Errorf("want 1 to %d minutes, got %d", 7*24*60, n)
	}
	return time.Duration(n) * time.Minute, nil
}

func boolean(v string) (bool, error) {
	switch v {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("want true or false, got %s", v)
}

// stringArray reads ["a", 'b', …], a trailing comma allowed.
func stringArray(v string) ([]string, error) {
	if !strings.HasPrefix(v, "[") || !strings.HasSuffix(v, "]") {
		return nil, fmt.Errorf("want an array of strings like [\"w1:p2\"], got %s", v)
	}
	rest := strings.TrimSpace(v[1 : len(v)-1])
	out := []string{}
	for rest != "" {
		s, tail, err := str(rest)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
		tail = strings.TrimSpace(tail)
		if tail != "" && tail[0] != ',' {
			return nil, fmt.Errorf("want a comma between strings, got %s", tail)
		}
		rest = strings.TrimSpace(strings.TrimPrefix(tail, ","))
	}
	return out, nil
}

// str reads one quoted string at the start of v and returns what follows.
func str(v string) (string, string, error) {
	switch v[0] {
	case '\'':
		if end := strings.IndexByte(v[1:], '\''); end >= 0 {
			return v[1 : end+1], v[end+2:], nil
		}
	case '"':
		for i := 1; i < len(v); i++ {
			if v[i] == '\\' {
				i++
				continue
			}
			if v[i] == '"' {
				s, err := strconv.Unquote(v[:i+1])
				if err != nil {
					return "", "", fmt.Errorf("bad string %s", v[:i+1])
				}
				return s, v[i+1:], nil
			}
		}
	default:
		return "", "", fmt.Errorf("want a quoted string, got %s", v)
	}
	return "", "", fmt.Errorf("unterminated string %s", v)
}
