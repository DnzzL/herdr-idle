package sweep

import (
	"path/filepath"
	"strings"

	"github.com/DnzzL/herdr-idle/internal/herdr"
)

// languageServers are language servers whose name carries no generic marker.
var languageServers = map[string]bool{
	"gopls": true, "rust-analyzer": true, "clangd": true, "tsserver": true, "jdtls": true,
}

// withChildren returns the foreground processes, each missing command line
// taken from the OS table (a process that rewrites its title has none in
// herdr), followed by the agent's direct children from that table, whatever
// their group.
func withChildren(foreground []herdr.Process, agentPID int, table []OSProcess) []herdr.Process {
	argv := make(map[int][]string, len(table))
	for _, p := range table {
		argv[p.PID] = p.Argv
	}
	procs := make([]herdr.Process, 0, len(foreground))
	for _, p := range foreground {
		if len(p.Argv) == 0 {
			p.Argv = argv[p.PID]
		}
		procs = append(procs, p)
	}
	for _, p := range table {
		if p.PPID == agentPID {
			procs = append(procs, herdr.Process{PID: p.PID, Argv: p.Argv})
		}
	}
	return procs
}

// busyChild returns the first process, other than the agent itself, that
// is not one of its MCP or language servers: running work (a shell, a dev
// server, a test run) that SIGTERM would cut.
func busyChild(agentPID int, procs []herdr.Process) (herdr.Process, bool) {
	for _, p := range procs {
		if p.PID != agentPID && !isServer(p.Argv) {
			return p, true
		}
	}
	return herdr.Process{}, false
}

// isServer reports whether argv runs an MCP or language server. Only the
// file name of each path-like argument counts, so a project directory
// named "mcp-tools" does not make its dev server look like one. A word is
// a run of letters and digits.
//
//   - MCP: a word "mcp" (codebase-memory-mcp, mcp-server-fetch,
//     "cli.js mcp start"), or an @modelcontextprotocol/ package.
//   - Language server: a word "lsp" or "langserver", "language-server" in
//     the name, or a well-known name (gopls, rust-analyzer, …).
func isServer(argv []string) bool {
	for _, arg := range argv {
		arg = strings.ToLower(arg)
		if strings.HasPrefix(arg, "@modelcontextprotocol/") {
			return true
		}
		name := filepath.Base(arg)
		if languageServers[name] || strings.Contains(name, "language-server") {
			return true
		}
		words := strings.FieldsFunc(name, func(r rune) bool {
			return (r < 'a' || r > 'z') && (r < '0' || r > '9')
		})
		for _, w := range words {
			if w == "mcp" || w == "lsp" || w == "langserver" {
				return true
			}
		}
	}
	return false
}
