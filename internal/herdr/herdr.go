// Package herdr is a client for herdr's JSON socket API: one JSON line per
// request, one JSON line back. It carries only the calls herdr-idle makes.
package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"sync/atomic"
)

// Agent is one agent pane from session.snapshot.
type Agent struct {
	PaneID         string        `json:"pane_id"`
	Agent          string        `json:"agent"`
	Status         string        `json:"agent_status"`
	Session        *AgentSession `json:"agent_session"`
	Focused        bool          `json:"focused"`
	StateChangeSeq uint64        `json:"state_change_seq"`
	Cwd            string        `json:"cwd"`
}

// AgentSession is herdr's session ref for the agent in a pane. Kind is "id"
// or "path"; Value is what that agent's resume flag takes.
type AgentSession struct {
	Source string `json:"source"`
	Agent  string `json:"agent"`
	Kind   string `json:"kind"`
	Value  string `json:"value"`
}

// ProcessInfo is pane.process_info: the pane's shell and the process group in
// its terminal's foreground.
type ProcessInfo struct {
	ShellPID          int       `json:"shell_pid"`
	ForegroundGroupID int       `json:"foreground_process_group_id"`
	Foreground        []Process `json:"foreground_processes"`
}

// Process is one member of the foreground process group.
type Process struct {
	PID  int      `json:"pid"`
	Argv []string `json:"argv"`
}

// AgentReport is pane.report_agent. State is idle|working|blocked|unknown.
type AgentReport struct {
	PaneID string `json:"pane_id"`
	Source string `json:"source"`
	Agent  string `json:"agent"`
	State  string `json:"state"`
}

// MetadataReport is pane.report_metadata, limited to the fields herdr-idle
// sets.
type MetadataReport struct {
	PaneID          string            `json:"pane_id"`
	Source          string            `json:"source"`
	AppliesToSource string            `json:"applies_to_source,omitempty"`
	DisplayAgent    string            `json:"display_agent,omitempty"`
	StateLabels     map[string]string `json:"state_labels,omitempty"`
}

// Error is an error response from herdr.
type Error struct {
	Method  string
	Code    string
	Message string
}

func (e *Error) Error() string {
	return "herdr " + e.Method + ": " + e.Code + ": " + e.Message
}

// Client talks to one herdr server. Every call dials the socket afresh.
type Client struct {
	socket string
	seq    atomic.Uint64
}

// New returns a client for the herdr server listening on socket.
func New(socket string) *Client {
	return &Client{socket: socket}
}

// Snapshot returns every agent pane in the session.
func (c *Client) Snapshot(ctx context.Context) ([]Agent, error) {
	var res struct {
		Snapshot struct {
			Agents []Agent `json:"agents"`
		} `json:"snapshot"`
	}
	if err := c.call(ctx, "session.snapshot", struct{}{}, &res); err != nil {
		return nil, err
	}
	return res.Snapshot.Agents, nil
}

// ProcessInfo returns the pane's shell pid and foreground process group.
func (c *Client) ProcessInfo(ctx context.Context, paneID string) (ProcessInfo, error) {
	var res struct {
		ProcessInfo ProcessInfo `json:"process_info"`
	}
	err := c.call(ctx, "pane.process_info", map[string]string{"pane_id": paneID}, &res)
	return res.ProcessInfo, err
}

// ReportAgent reports an agent for a pane on behalf of a plugin source.
func (c *Client) ReportAgent(ctx context.Context, r AgentReport) error {
	return c.call(ctx, "pane.report_agent", r, nil)
}

// ReportMetadata sets display metadata (shown agent, state labels) on a pane.
func (c *Client) ReportMetadata(ctx context.Context, r MetadataReport) error {
	return c.call(ctx, "pane.report_metadata", r, nil)
}

// SendText types text into the pane without pressing Enter.
func (c *Client) SendText(ctx context.Context, paneID, text string) error {
	return c.call(ctx, "pane.send_text", map[string]string{"pane_id": paneID, "text": text}, nil)
}

func (c *Client) call(ctx context.Context, method string, params, out any) error {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.socket)
	if err != nil {
		return fmt.Errorf("herdr %s: %w", method, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		if err := conn.SetDeadline(deadline); err != nil {
			return fmt.Errorf("herdr %s: %w", method, err)
		}
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()

	req, err := json.Marshal(map[string]any{
		"id":     strconv.FormatUint(c.seq.Add(1), 10),
		"method": method,
		"params": params,
	})
	if err != nil {
		return fmt.Errorf("herdr %s: %w", method, err)
	}
	if _, err := conn.Write(append(req, '\n')); err != nil {
		return fmt.Errorf("herdr %s: %w", method, err)
	}
	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return fmt.Errorf("herdr %s: %w", method, err)
	}
	var resp struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(line, &resp); err != nil {
		return fmt.Errorf("herdr %s: decode response: %w", method, err)
	}
	if resp.Error != nil {
		return &Error{Method: method, Code: resp.Error.Code, Message: resp.Error.Message}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(resp.Result, out); err != nil {
		return fmt.Errorf("herdr %s: decode result: %w", method, err)
	}
	return nil
}
