// herdr-idle puts idle agent panes asleep in herdr: the agent is stopped to
// free its memory, the pane stays in place with the resume command
// prefilled.
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/DnzzL/herdr-idle/internal/config"
	"github.com/DnzzL/herdr-idle/internal/herdr"
	"github.com/DnzzL/herdr-idle/internal/sweep"
)

// version is stamped by the release build; "dev" for local builds.
var version = "dev"

const usage = `herdr-idle — put idle agent panes asleep

Usage:
  herdr-idle sweep         Put every qualifying agent pane asleep, once
  herdr-idle daemon        Sweep every interval_minutes until the plugin is
                           disabled or herdr stops (herdr startup hook)
  herdr-idle preview       List every agent pane and why it would or would not
                           sleep; changes nothing (--hold: wait for Enter)
  herdr-idle config        Open config.toml in $VISUAL or $EDITOR, creating it
                           with commented defaults
  herdr-idle forget-closed Forget panes herdr no longer has (herdr pane, tab and
                           workspace close hook)
  herdr-idle ensure-daemon Start the daemon unless one runs (herdr pane focus
                           hook); sweep, preview, config and forget-closed do too
  herdr-idle version       Print the version
`

// errUsage means the command line was wrong; usage has been printed.
var errUsage = errors.New("usage")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Getenv, os.Stdin, os.Stdout)
	stop()
	switch {
	case errors.Is(err, errUsage):
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	case err != nil:
		fmt.Fprintln(os.Stderr, "herdr-idle:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, getenv func(string) string, stdin io.Reader, stdout io.Writer) error {
	// herdr starts the daemon only with its server: every other plugin
	// command starts it when none runs, so a fresh install sweeps without a
	// herdr restart.
	if len(args) > 0 && slices.Contains([]string{"sweep", "preview", "config", "forget-closed", "ensure-daemon"}, args[0]) {
		if err := ensureDaemon(getenv); err != nil {
			fmt.Fprintln(os.Stderr, "herdr-idle: start daemon:", err)
		}
	}
	switch {
	case len(args) == 1 && args[0] == "ensure-daemon":
		return nil
	case len(args) == 1 && args[0] == "version":
		_, err := fmt.Fprintln(stdout, version)
		return err
	case len(args) == 1 && args[0] == "sweep":
		s, err := newSweeper(getenv)
		if err != nil {
			return err
		}
		outcomes, err := s.Run(ctx)
		printOutcomes(stdout, outcomes, err)
		return err
	case len(args) == 1 && args[0] == "daemon":
		return runDaemon(ctx, getenv, stdout)
	case len(args) == 1 && args[0] == "preview":
		return runPreview(ctx, getenv, false, stdin, stdout)
	case len(args) == 2 && args[0] == "preview" && args[1] == "--hold":
		return runPreview(ctx, getenv, true, stdin, stdout)
	case len(args) == 1 && args[0] == "config":
		return editConfig(ctx, getenv, stdin, stdout)
	case len(args) == 1 && args[0] == "forget-closed":
		return runForgetClosed(ctx, getenv)
	default:
		return errUsage
	}
}

// runPreview prints what a sweep would do. With hold, it waits for Enter
// and reports any error on screen: an overlay pane closes with its command,
// so it stays open until the user has read it.
func runPreview(ctx context.Context, getenv func(string) string, hold bool, stdin io.Reader, stdout io.Writer) error {
	s, err := newSweeper(getenv)
	if err == nil {
		var outcomes []sweep.Outcome
		outcomes, err = s.Preview(ctx)
		printOutcomes(stdout, outcomes, err)
	}
	if !hold {
		return err
	}
	if err != nil {
		fmt.Fprintln(stdout, "\nherdr-idle:", err)
	}
	fmt.Fprint(stdout, "\nPress Enter to close.")
	_, _ = bufio.NewReader(stdin).ReadString('\n')
	return nil
}

func newSweeper(getenv func(string) string) (sweep.Sweeper, error) {
	socket, statePath, err := sessionPaths(getenv)
	if err != nil {
		return sweep.Sweeper{}, err
	}
	cfgPath, err := configPath(getenv)
	if err != nil {
		return sweep.Sweeper{}, err
	}
	return sweep.Sweeper{
		Herdr:     herdr.New(socket),
		Signaler:  groupSignaler{},
		Processes: psProcesses{},
		StatePath: statePath,
		// Screens saved before sleep, one directory per session like the state.
		CaptureDir: strings.TrimSuffix(statePath, ".json") + "-screens",
		Config:     &config.File{Path: cfgPath},
	}, nil
}

// sessionPaths returns the herdr session's socket and the plugin's state
// file for that session.
func sessionPaths(getenv func(string) string) (socket, statePath string, err error) {
	socket = getenv("HERDR_SOCKET_PATH")
	if socket == "" {
		return "", "", errors.New("HERDR_SOCKET_PATH is not set: run inside herdr")
	}
	dir, err := stateDir(getenv)
	if err != nil {
		return "", "", err
	}
	return socket, filepath.Join(dir, stateFile(socket)), nil
}

// runDaemon is herdr's startup hook, run on every server start. A second
// daemon for the same session leaves quietly.
func runDaemon(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	s, err := newSweeper(getenv)
	if err != nil {
		return err
	}
	// herdr pipes stdout and stops reading when its server stops, but the
	// daemon outlives a quick restart and keeps serving the new server: a
	// log line must not kill it with SIGPIPE.
	signal.Ignore(syscall.SIGPIPE)
	// The daemon logs to its own file: one started by ensureDaemon has no
	// output herdr reads.
	logPath := filepath.Join(filepath.Dir(s.StatePath), "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	d := sweep.Daemon{
		Sweeper: s,
		Plugin:  herdr.New(getenv("HERDR_SOCKET_PATH")),
		Log: func(outcomes []sweep.Outcome, err error) {
			fmt.Fprintln(logFile, time.Now().Format(time.RFC3339))
			printOutcomes(logFile, outcomes, err)
			if err != nil {
				fmt.Fprintln(logFile, "error:", err)
			}
		},
		Upgraded: binaryReplaced(exe),
		Reexec: func() error {
			return syscall.Exec(exe, os.Args, append(os.Environ(), reexecEnv+"=1"))
		},
		KeepClocks: getenv(reexecEnv) == "1",
	}
	err = d.Run(ctx)
	if errors.Is(err, sweep.ErrAlreadyRunning) {
		_, err = fmt.Fprintln(stdout, "herdr-idle:", err)
	}
	return err
}

// reexecEnv marks a daemon that re-executed itself on a new binary.
const reexecEnv = "HERDR_IDLE_REEXEC"

// binaryReplaced reports whether the file at exe changed since it last
// returned true, or since it was made; a binary it cannot stat has not.
func binaryReplaced(exe string) func() bool {
	stamp := func() string {
		st, err := os.Stat(exe)
		if err != nil {
			return ""
		}
		return fmt.Sprintf("%d-%d", st.ModTime().UnixNano(), st.Size())
	}
	last := stamp()
	return func() bool {
		now := stamp()
		if now == "" || now == last {
			return false
		}
		last = now
		return true
	}
}

// ensureDaemon starts a daemon for this herdr session unless one runs. It
// runs in a session of its own with no pipe back to herdr: herdr counts a
// plugin command as running until its output closes.
func ensureDaemon(getenv func(string) string) error {
	_, statePath, err := sessionPaths(getenv)
	if err != nil {
		return err
	}
	return sweep.EnsureDaemon(statePath, func() error {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		cmd := exec.Command(exe, "daemon")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err := cmd.Start(); err != nil {
			return err
		}
		return cmd.Process.Release()
	})
}

// printOutcomes prints one line per agent pane.
func printOutcomes(w io.Writer, outcomes []sweep.Outcome, err error) {
	for _, o := range outcomes {
		fmt.Fprintf(w, "%s\t%s\t%s\n", o.PaneID, o.Agent, o.Reason)
	}
	if len(outcomes) == 0 && err == nil {
		fmt.Fprintln(w, "No agent panes.")
	}
}

// editConfig opens the config file in the user's editor, creating it with
// commented defaults first, then shows any problem in what was saved.
func editConfig(ctx context.Context, getenv func(string) string, stdin io.Reader, stdout io.Writer) error {
	path, err := configPath(getenv)
	if err != nil {
		return err
	}
	if err := createConfig(path); err != nil {
		return err
	}
	editor := getenv("VISUAL")
	if editor == "" {
		editor = getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	// $EDITOR may carry flags ("code --wait"): let the shell split it.
	cmd := exec.CommandContext(ctx, "sh", "-c", editor+` "$1"`, "sh", path)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor %s: %w", editor, err)
	}
	if _, _, err := (&config.File{Path: path}).Load(); err != nil {
		fmt.Fprintf(stdout, "\n%v\n\nUntil this is fixed, the daemon keeps its last valid config and other sweeps do nothing. Press Enter to close.", err)
		_, _ = bufio.NewReader(stdin).ReadString('\n')
	}
	return nil
}

// createConfig writes the commented defaults unless the file exists.
func createConfig(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = f.WriteString(config.Template)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	return nil
}

// configPath is config.toml in the plugin config directory herdr hands its
// plugin commands, or the same directory derived the way herdr does when run
// from a shell.
func configPath(getenv func(string) string) (string, error) {
	dir := getenv("HERDR_PLUGIN_CONFIG_DIR")
	if dir == "" {
		base := getenv("XDG_CONFIG_HOME")
		if base == "" {
			home := getenv("HOME")
			if home == "" {
				return "", errors.New("cannot locate the config directory: HERDR_PLUGIN_CONFIG_DIR and HOME are unset")
			}
			base = filepath.Join(home, ".config")
		}
		dir = filepath.Join(base, "herdr", "plugins", "config", sweep.PluginID)
	}
	return filepath.Join(dir, "config.toml"), nil
}

// runForgetClosed is herdr's pane.closed, tab.closed and workspace.closed
// hook.
func runForgetClosed(ctx context.Context, getenv func(string) string) error {
	socket, statePath, err := sessionPaths(getenv)
	if err != nil {
		return err
	}
	s := sweep.Sweeper{Herdr: herdr.New(socket), StatePath: statePath}
	return s.ForgetClosed(ctx)
}

// stateDir is the plugin state directory herdr hands its plugin commands, or
// the same directory derived the way herdr does when run from a shell.
func stateDir(getenv func(string) string) (string, error) {
	if dir := getenv("HERDR_PLUGIN_STATE_DIR"); dir != "" {
		return dir, nil
	}
	base := getenv("XDG_STATE_HOME")
	if base == "" {
		home := getenv("HOME")
		if home == "" {
			return "", errors.New("cannot locate the state directory: HERDR_PLUGIN_STATE_DIR and HOME are unset")
		}
		base = filepath.Join(home, ".local", "state")
	}
	return filepath.Join(base, "herdr", "plugins", sweep.PluginID), nil
}

// stateFile names one state file per herdr session: pane ids are only
// unique within a session.
func stateFile(socket string) string {
	sum := sha256.Sum256([]byte(socket))
	return fmt.Sprintf("state-%x.json", sum[:6])
}

// groupSignaler signals real process groups.
type groupSignaler struct{}

func (groupSignaler) Terminate(pgid int) error {
	// kill(-1) signals every process the user owns, kill(0) our own group.
	if pgid <= 1 {
		return fmt.Errorf("refusing to signal process group %d", pgid)
	}
	return syscall.Kill(-pgid, syscall.SIGTERM)
}
