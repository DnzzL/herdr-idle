// herdr-idle puts idle agent panes asleep in herdr: the agent is stopped to
// free its memory, the pane stays in place with the resume command
// prefilled.
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/DnzzL/herdr-idle/internal/herdr"
	"github.com/DnzzL/herdr-idle/internal/sweep"
)

// version is stamped by the release build; "dev" for local builds.
var version = "dev"

const usage = `herdr-idle — put idle agent panes asleep

Usage:
  herdr-idle sweep     Put every qualifying agent pane asleep, once
  herdr-idle version   Print the version
`

// idleAfter is how long a pane must stay quiet before it is put asleep.
const idleAfter = 60 * time.Minute

// errUsage means the command line was wrong; usage has been printed.
var errUsage = errors.New("usage")

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Getenv, os.Stdout)
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

func run(ctx context.Context, args []string, getenv func(string) string, stdout io.Writer) error {
	if len(args) != 1 {
		return errUsage
	}
	switch args[0] {
	case "version":
		_, err := fmt.Fprintln(stdout, version)
		return err
	case "sweep":
		return runSweep(ctx, getenv, stdout)
	default:
		return errUsage
	}
}

func runSweep(ctx context.Context, getenv func(string) string, stdout io.Writer) error {
	socket := getenv("HERDR_SOCKET_PATH")
	if socket == "" {
		return errors.New("HERDR_SOCKET_PATH is not set: run inside herdr")
	}
	stateDir, err := stateDir(getenv)
	if err != nil {
		return err
	}
	s := sweep.Sweeper{
		Herdr:     herdr.New(socket),
		Signaler:  groupSignaler{},
		Processes: psProcesses{},
		StatePath: filepath.Join(stateDir, stateFile(socket)),
		// Screens saved before sleep, one directory per session like the state.
		CaptureDir: filepath.Join(stateDir, strings.TrimSuffix(stateFile(socket), ".json")+"-screens"),
		IdleAfter:  idleAfter,
	}
	outcomes, err := s.Run(ctx)
	for _, o := range outcomes {
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", o.PaneID, o.Agent, o.Reason)
	}
	return err
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
	return filepath.Join(base, "herdr", "plugins", "dnzzl.idle"), nil
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
