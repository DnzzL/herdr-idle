package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/DnzzL/herdr-idle/internal/herdr"
)

// psProcesses reads the process table with ps(1), the same on macOS and
// Linux. Zombies are left out.
type psProcesses struct{}

func (psProcesses) Children(ctx context.Context, pid int) ([]herdr.Process, error) {
	out, err := exec.CommandContext(ctx, "ps", "-A", "-o", "pid=,ppid=,stat=,args=").Output()
	if err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	var children []herdr.Process
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(nil, 1<<20) // args lines can be long
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		// A zombie has finished: it runs nothing.
		if len(f) < 4 || strings.HasPrefix(f[2], "Z") {
			continue
		}
		ppid, err := strconv.Atoi(f[1])
		if err != nil || ppid != pid {
			continue
		}
		child, err := strconv.Atoi(f[0])
		if err != nil {
			continue
		}
		// ps joins argv with spaces; splitting them back is lossy but
		// enough to tell what a process runs.
		children = append(children, herdr.Process{PID: child, Argv: f[3:]})
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("list processes: %w", err)
	}
	return children, nil
}
