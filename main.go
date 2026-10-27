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
	"syscall"

	"github.com/DnzzL/herdr-idle/internal/config"
	"github.com/DnzzL/herdr-idle/internal/herdr"
	"github.com/DnzzL/herdr-idle/internal/sweep"
)

// version is stamped by the release build; "dev" for local builds.
var version = "dev"

const usage = `herdr-idle — put idle agent panes asleep

Usage:
  herdr-idle sweep         Put every qualifying agent pane asleep, once
  herdr-idle preview       List every agent pane and why it would or would not
                           sleep; changes nothing (--hold: wait for Enter)
  herdr-idle config        Open config.toml in $VISUAL or $EDITOR, creating it
                           with commented defaults
  herdr-idle pane-closed   Forget the pane in HERDR_PANE_ID (herdr pane.closed hook)
  herdr-idle version       Print the version
`

// pluginID must match the id in herdr-plugin.toml.
const pluginID = "dnzzl.idle"

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
	switch {
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
	case len(args) == 1 && args[0] == "preview":
		s, err := newSweeper(getenv)
		if err != nil {
			return err
		}
		outcomes, err := s.Preview(ctx)
		printOutcomes(stdout, outcomes, err)
		return err
	case len(args) == 2 && args[0] == "preview" && args[1] == "--hold":
		// An overlay pane closes with its command: keep it open until the
		// user has read it.
		s, err := newSweeper(getenv)
		if err == nil {
			var outcomes []sweep.Outcome
			outcomes, err = s.Preview(ctx)
			printOutcomes(stdout, outcomes, err)
		}
		if err != nil {
			fmt.Fprintln(stdout, "\nherdr-idle:", err)
		}
		fmt.Fprint(stdout, "\nPress Enter to close.")
		_, _ = bufio.NewReader(stdin).ReadString('\n')
		return nil
	case len(args) == 1 && args[0] == "config":
		return editConfig(ctx, getenv, stdin, stdout)
	case len(args) == 1 && args[0] == "pane-closed":
		return runPaneClosed(ctx, getenv)
	default:
		return errUsage
	}
}

func newSweeper(getenv func(string) string) (sweep.Sweeper, error) {
	socket := getenv("HERDR_SOCKET_PATH")
	if socket == "" {
		return sweep.Sweeper{}, errors.New("HERDR_SOCKET_PATH is not set: run inside herdr")
	}
	stateDir, err := stateDir(getenv)
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
		StatePath: filepath.Join(stateDir, stateFile(socket)),
		Config:    &config.File{Path: cfgPath},
	}, nil
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
		dir = filepath.Join(base, "herdr", "plugins", "config", pluginID)
	}
	return filepath.Join(dir, "config.toml"), nil
}

// runPaneClosed is herdr's pane.closed hook: herdr passes the closed pane in
// HERDR_PANE_ID.
func runPaneClosed(ctx context.Context, getenv func(string) string) error {
	socket := getenv("HERDR_SOCKET_PATH")
	if socket == "" {
		return errors.New("HERDR_SOCKET_PATH is not set: run inside herdr")
	}
	paneID := getenv("HERDR_PANE_ID")
	if paneID == "" {
		return errors.New("HERDR_PANE_ID is not set: run from herdr's pane.closed hook")
	}
	stateDir, err := stateDir(getenv)
	if err != nil {
		return err
	}
	s := sweep.Sweeper{StatePath: filepath.Join(stateDir, stateFile(socket))}
	return s.PaneClosed(ctx, paneID)
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
	return filepath.Join(base, "herdr", "plugins", pluginID), nil
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
