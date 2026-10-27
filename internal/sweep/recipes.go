package sweep

// flagRef resumes with `flag ref`.
func flagRef(flag string) func(string) []string {
	return func(ref string) []string { return []string{flag, ref} }
}

var idRef = []string{"id"}

// recipes is keyed by herdr agent kind. Resume tokens come from herdr 0.9.5
// src/agent_resume.rs `plan`; each kind's option lists name their source.
var recipes = map[string]recipe{
	// claude --help, Claude Code 2.1.311.
	"claude": {
		command:  "claude",
		names:    []string{"claude-code"},
		refKinds: idRef,
		resume:   flagRef("--resume"),
		values: map[string]arity{
			"--add-dir": manyValues, "--agent": oneValue, "--agents": oneValue,
			"--allowedTools": manyValues, "--allowed-tools": manyValues,
			"--append-system-prompt": oneValue, "--autocompact": oneValue,
			"--betas": manyValues, "--cloud": optionalValue,
			"-d": optionalValue, "--debug": optionalValue, "--debug-file": oneValue,
			"--disallowedTools": manyValues, "--disallowed-tools": manyValues,
			"--effort": oneValue, "--environment": oneValue,
			"--fallback-model": oneValue, "--file": manyValues,
			"--input-format": oneValue, "--json-schema": oneValue,
			"--max-budget-usd": oneValue, "--mcp-config": manyValues,
			"--model": oneValue, "-n": oneValue, "--name": oneValue,
			"--output-format": oneValue, "--permission-mode": oneValue,
			"--permission-prompts": oneValue, "--plugin-dir": oneValue,
			"--plugin-url": oneValue, "--prompt-suggestions": optionalValue,
			"--remote-control": optionalValue,
			"--remote-control-session-name-prefix": oneValue,
			"--setting-sources": oneValue, "--settings": oneValue,
			"--system-prompt": oneValue, "--system-prompt-snapshot": oneValue,
			"--tools": manyValues, "-w": optionalValue, "--worktree": optionalValue,
		},
		drop: map[string]arity{
			"--resume": optionalValue, "-r": optionalValue,
			"--continue": noValue, "-c": noValue,
			"--session-id": oneValue, "--fork-session": noValue,
			"--from-pr": optionalValue, "--teleport": optionalValue,
		},
	},
}
