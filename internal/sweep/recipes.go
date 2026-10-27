package sweep

import (
	"strings"

	"github.com/DnzzL/herdr-idle/internal/herdr"
)

// opts maps option names to their arity.
type opts map[string]arity

// with returns o plus names, each of arity n.
func (o opts) with(n arity, names ...string) opts {
	for _, name := range names {
		o[name] = n
	}
	return o
}

// flagRef resumes with `flag ref`.
func flagRef(flag string) func(string) []string {
	return func(ref string) []string { return []string{flag, ref} }
}

// inlineRef resumes with `flag=ref`.
func inlineRef(flag string) func(string) []string {
	return func(ref string) []string { return []string{flag + "=" + ref} }
}

var (
	idRef       = []string{"id"}
	pathOrIDRef = []string{"path", "id"}
)

// recipes is keyed by herdr agent kind. Commands, aliases and resume tokens
// come from herdr 0.9.5 (src/agent_resume.rs `plan`, src/detect/mod.rs).
// Each kind's option lists name their source; prompt options are dropped
// like positional prompts.
var recipes = map[herdr.Kind]recipe{
	// `claude --help`, Claude Code 2.1.311.
	"claude": {
		command:  "claude",
		names:    []string{"claude-code"},
		refKinds: idRef,
		resume:   flagRef("--resume"),
		values: opts{}.
			with(oneValue, "--agent", "--agents", "--append-system-prompt",
				"--autocompact", "--debug-file", "--effort", "--environment",
				"--fallback-model", "--input-format", "--json-schema",
				"--max-budget-usd", "--model", "-n", "--name", "--output-format",
				"--permission-mode", "--permission-prompts", "--plugin-dir",
				"--plugin-url", "--remote-control-session-name-prefix",
				"--setting-sources", "--settings", "--system-prompt",
				"--system-prompt-snapshot").
			with(optionalValue, "--cloud", "-d", "--debug", "--prompt-suggestions",
				"--remote-control", "-w", "--worktree").
			with(manyValues, "--add-dir", "--allowedTools", "--allowed-tools",
				"--betas", "--disallowedTools", "--disallowed-tools", "--file",
				"--mcp-config", "--tools"),
		drop: opts{}.
			with(noValue, "--continue", "-c", "--fork-session").
			with(oneValue, "--session-id").
			with(optionalValue, "--resume", "-r", "--from-pr", "--teleport"),
	},
	// openai/codex codex-rs/cli/src/main.rs and tui/src/cli.rs, 0.164.0.
	// Resume is a subcommand: the old `resume <id>` words are positionals.
	"codex": {
		command:  "codex",
		refKinds: idRef,
		resume:   func(ref string) []string { return []string{"resume", ref} },
		values: opts{}.
			with(oneValue, "-m", "--model", "-p", "--profile", "-s", "--sandbox",
				"-a", "--ask-for-approval", "-C", "--cd", "--add-dir",
				"--local-provider", "-c", "--config", "--enable", "--disable",
				"--remote", "--remote-auth-token-env").
			with(manyValues, "-i", "--image"),
		drop: opts{}.with(noValue, "--last", "--all", "--include-non-interactive"),
	},
	// github/docs copilot-cli-reference/cli-command-reference.md (1.0.104).
	"copilot": {
		command:  "copilot",
		names:    []string{"github-copilot", "ghcs"},
		refKinds: idRef,
		resume:   inlineRef("--resume"),
		values: opts{}.
			with(oneValue, "-C", "--model", "--effort", "--reasoning-effort",
				"--mode", "--agent", "--context", "--output-format", "--log-dir",
				"--log-level", "--stream", "--add-dir", "--config-dir",
				"--plugin-dir", "--additional-mcp-config", "--attachment",
				"--allow-tool", "--deny-tool", "--allow-url", "--deny-url",
				"--available-tools", "--excluded-tools", "--add-github-mcp-tool",
				"--add-github-mcp-toolset", "--enable-mcp-server",
				"--disable-mcp-server", "--secret-env-vars", "--share",
				"--max-ai-credits", "--max-autopilot-continues",
				"--extension-sdk-path").
			with(optionalValue, "--mouse", "--sync-session"),
		drop: opts{}.
			with(noValue, "--continue").
			with(oneValue, "--session-id", "-i", "--interactive", "-p", "--prompt",
				"-n", "--name").
			with(optionalValue, "-r", "--resume", "--connect", "-w", "--worktree"),
	},
	// docs.devin.ai/cli/reference/commands.md (no version published).
	"devin": {
		command:  "devin",
		names:    []string{"devin-cli"},
		refKinds: idRef,
		resume:   flagRef("--resume"),
		values: opts{}.
			with(oneValue, "--model", "--permission-mode", "--config").
			with(optionalValue, "--export", "--respect-workspace-trust"),
		drop: opts{}.
			with(noValue, "-c", "--continue").
			with(oneValue, "--prompt-file").
			with(optionalValue, "-r", "--resume", "-p", "--print"),
	},
	// `droid --help`, Factory droid 0.164.0.
	"droid": {
		command:  "droid",
		refKinds: idRef,
		resume:   flagRef("--resume"),
		values: opts{}.
			with(oneValue, "--settings", "--append-system-prompt",
				"--append-system-prompt-file", "--cwd", "--worktree-dir", "--auto").
			with(optionalValue, "-w", "--worktree"),
		drop: opts{}.
			with(oneValue, "--fork").
			with(optionalValue, "-r", "--resume"),
	},
	// MoonshotAI/kimi-cli src/kimi_cli/cli/__init__.py, 1.52.0.
	"kimi": {
		command:  "kimi",
		names:    []string{"kimi-code"},
		packages: []string{"/node_modules/@moonshot-ai/kimi-code/dist/main.mjs"},
		refKinds: idRef,
		resume:   flagRef("--session"),
		values: opts{}.
			with(oneValue, "-w", "--work-dir", "--add-dir", "-m", "--model",
				"--config", "--config-file", "--input-format", "--output-format",
				"--agent", "--agent-file", "--mcp-config-file", "--mcp-config",
				"--skills-dir", "--max-steps-per-turn", "--max-retries-per-step",
				"--max-ralph-iterations"),
		drop: opts{}.
			with(noValue, "-C", "--continue").
			with(oneValue, "-p", "--prompt", "-c", "--command").
			with(optionalValue, "-S", "--session", "--resume", "-r"),
	},
	// npm mastracode 0.48.0 dist/cli.js and @mastra/code-sdk 1.15.0
	// dist/headless/flags.js (bundled builds).
	"mastracode": {
		command:  "mastracode",
		names:    []string{"mastra-code"},
		packages: []string{"/node_modules/mastracode/dist/cli"},
		refKinds: idRef,
		resume:   flagRef("--thread"),
		values: opts{}.
			with(oneValue, "--title", "--resource-id", "--timeout", "--max-turns",
				"--permission-mode", "-o", "--output", "-m", "--model", "--pack",
				"--mode", "--thinking-level", "--settings"),
		drop: opts{}.
			with(noValue, "-c", "--continue", "--clone-thread").
			with(oneValue, "-t", "--thread", "-p", "--prompt",
				"--tui-initial-prompt", "--tui-prompt"),
	},
	// `pi --help`, pi 1.2.0.
	"pi": {
		command: "pi",
		packages: []string{
			"/node_modules/@earendil-works/pi-coding-agent/dist/cli.js",
			"/node_modules/@earendil-works/pi-coding-agent/dist/bundle/cli.js",
		},
		refKinds: pathOrIDRef,
		resume:   flagRef("--session"),
		values: opts{}.
			with(oneValue, "--provider", "--model", "--api-key", "--system-prompt",
				"--append-system-prompt", "--mode", "--session-dir", "-n", "--name",
				"--models", "-t", "--tools", "-xt", "--exclude-tools", "--thinking",
				"-e", "--extension", "--skill", "--prompt-template", "--theme",
				"--use-theme", "--export", "--tui-mode", "--no-tool").
			with(optionalValue, "--list-models"),
		drop: opts{}.
			with(noValue, "-c", "--continue", "-r", "--resume", "--no-session").
			with(oneValue, "--session", "--session-id", "--fork"),
	},
	// npm @oh-my-pi/pi-coding-agent 18.13.1 src/cli/flag-tables.ts.
	"omp": {
		command:  "omp",
		packages: []string{"/node_modules/@oh-my-pi/pi-coding-agent/dist/cli.js"},
		refKinds: pathOrIDRef,
		resume:   inlineRef("--resume"),
		values: opts{}.
			with(oneValue, "--cwd", "--config", "--add-dir", "--mode", "--model",
				"--smol", "--slow", "--provider", "--plan", "--prewalk-into",
				"--plan-yolo-into", "--max-time", "--warn-cost", "--max-cost",
				"--service-tier", "--api-key", "--system-prompt",
				"--system-prompt-template", "--append-system-prompt",
				"--prompt-cache-key", "--session-dir", "--models", "--tools",
				"--thinking", "--export", "--hook", "-e", "--extension",
				"--trusted-extension", "--plugin-dir", "--skills",
				"--approval-mode", "--profile", "--alias"),
		drop: opts{}.
			with(noValue, "-c", "--continue", "--from-claude", "--from-codex",
				"--no-session").
			with(oneValue, "--fork", "--goal", "--provider-session-id").
			with(optionalValue, "-r", "--resume", "--restore", "--session"),
	},
	// NousResearch/hermes-agent hermes_cli/_parser.py (main branch).
	"hermes": {
		command:  "hermes",
		names:    []string{"hermes-agent"},
		refKinds: idRef,
		resume:   flagRef("--resume"),
		values: opts{}.
			with(oneValue, "--usage-file", "-m", "--model", "--provider",
				"--reasoning", "-t", "--toolsets", "--in", "-s", "--skills",
				"-p", "--profile", "--query-file", "--image", "--base-url",
				"--api-key", "--max-turns", "--run-budget", "--source", "--format"),
		drop: opts{}.
			with(noValue, "--create-if-missing").
			with(oneValue, "-r", "--resume", "-z", "--oneshot", "-q", "--query").
			with(optionalValue, "-c", "--continue"),
	},
	// `opencode --help`, opencode 1.19.0.
	"opencode": {
		command:  "opencode",
		names:    []string{"opencode2", "open-code"},
		refKinds: idRef,
		resume:   flagRef("--session"),
		values: opts{}.
			with(oneValue, "--log-level", "--port", "--hostname", "--mdns-domain",
				"-m", "--model", "--agent", "--replay-limit").
			with(manyValues, "--cors"),
		drop: opts{}.
			with(noValue, "-c", "--continue", "--fork").
			with(oneValue, "-s", "--session", "--prompt"),
	},
	// npm @qoder-ai/qodercli 1.1.79 bundle/qodercli.js (minified commander).
	"qodercli": {
		command:  "qodercli",
		names:    []string{"qoderclicn", "qoder", "qodercn"},
		refKinds: idRef,
		resume:   flagRef("--resume"),
		values: opts{}.
			with(oneValue, "-m", "--model", "-w", "--cwd", "--workspace",
				"--config-dir", "--permission-mode", "--allowed-tools",
				"--disallowed-tools", "--attachment", "-e", "--extensions",
				"--plugin-dir", "--add-dir", "--mcp-config", "--setting-sources",
				"--settings", "-o", "--output-format", "--input-format",
				"--max-output-tokens", "--max-turns", "--reasoning-effort",
				"--thinking", "--thinking-budget", "--context-window", "--agent",
				"--agents", "--system-prompt", "--system-prompt-file",
				"--append-system-prompt", "--output-style", "--proxy", "-n", "--name").
			with(optionalValue, "--worktree").
			with(manyValues, "--tools"),
		drop: opts{}.
			with(noValue, "-c", "--continue", "--fork-session", "--list-sessions",
				"--no-session-persistence").
			with(oneValue, "--prompt", "--session-id", "--remote-session",
				"--resume-session-at", "--resume-drops-turn", "--delete-session",
				"--remote-control").
			with(optionalValue, "-r", "--resume", "--remote", "-i",
				"--prompt-interactive"),
	},
	// QwenLM/qwen-code packages/cli/src/config/config.ts (yargs), 0.26.1.
	"qwen": {
		command:  "qwen",
		names:    []string{"qwen-code"},
		packages: []string{"/node_modules/@qwen-code/qwen-code/dist/index"},
		refKinds: idRef,
		resume:   flagRef("--resume"),
		values: opts{}.
			with(oneValue, "-m", "--model", "--advisor", "--system-prompt",
				"--append-system-prompt", "--output-style", "--sandbox-image",
				"--approval-mode", "--channel", "--mcp-config", "--openai-logging-dir",
				"--openai-api-key", "--openai-base-url", "-o", "--output-format",
				"--input-format", "--json-fd", "--json-file", "--json-schema",
				"--input-file", "--max-session-turns", "--max-wall-time",
				"--max-tool-calls", "--max-subagent-depth", "--core-tools",
				"--exclude-tools", "--disabled-slash-commands", "--auth-type",
				"--proxy", "--telemetry-target", "--telemetry-otlp-endpoint",
				"--telemetry-otlp-protocol", "--telemetry-outfile").
			with(optionalValue, "-s", "--sandbox").
			with(manyValues, "--fallback-model", "--allowed-mcp-server-names",
				"--disable-mcp-server", "--allowed-tools", "-e", "--extensions",
				"--include-directories", "--add-dir"),
		drop: opts{}.
			with(noValue, "-c", "--continue", "--fork-session").
			with(oneValue, "--session-id", "--sandbox-session-id", "-p", "--prompt",
				"-i", "--prompt-interactive").
			with(optionalValue, "-r", "--resume", "--worktree"),
	},
	// Kilo-Org/kilocode packages/opencode/src/cli/cmd/tui.ts (yargs), 7.8.18.
	"kilo": {
		command:  "kilo",
		names:    []string{"kilo-code"},
		refKinds: idRef,
		resume:   flagRef("--session"),
		values: opts{}.
			with(oneValue, "-m", "--model", "--agent", "--replay-limit", "--port",
				"--hostname", "--mdns-domain", "--log-level").
			with(manyValues, "--cors"),
		drop: opts{}.
			with(noValue, "-c", "--continue", "--fork", "--cloud-fork").
			with(oneValue, "-s", "--session", "--worktree", "--prompt"),
	},
	// cursor.com/docs/cli/reference/parameters.md. Its launcher runs
	// node index.js from cursor-agent/versions/<version>/.
	"cursor": {
		command:  "cursor-agent",
		names:    []string{"cursor"},
		packages: []string{"/cursor-agent/versions/"},
		refKinds: idRef,
		resume:   flagRef("--resume"),
		values: opts{}.
			with(oneValue, "--api-key", "-H", "--header", "--output-format",
				"--model", "--mode", "--sandbox", "--workspace", "--plugin-dir",
				"--worktree-base").
			with(optionalValue, "-w", "--worktree"),
		drop: opts{}.
			with(noValue, "--continue").
			with(optionalValue, "--resume"),
	},
	// antigravity.google/docs/cli/reference and google-antigravity/
	// antigravity-cli CHANGELOG.md (1.4.8); no parser source. -c is
	// --continue or --conversation depending on the page: either way it goes.
	"agy": {
		command:  "agy",
		names:    []string{"antigravity", "antigravity-cli"},
		refKinds: idRef,
		resume:   flagRef("--conversation"),
		values: opts{}.
			with(oneValue, "--mode", "--model", "--effort", "--agent", "--add-dir",
				"--output-format", "--input-format", "--json-schema",
				"--print-timeout", "--project", "--log-file"),
		drop: opts{}.
			with(noValue, "--continue").
			with(oneValue, "--conversation", "-i", "--prompt-interactive", "-p",
				"--print", "--prompt").
			with(optionalValue, "-c"),
	},
	// xai-org/grok-build crates/codegen/xai-grok-pager/src/app/cli.rs (clap).
	"grok": {
		command:  "grok",
		names:    []string{"grok-build"},
		refKinds: idRef,
		resume:   flagRef("--resume"),
		values: opts{}.
			with(oneValue, "-m", "--model", "--reasoning-effort", "--effort",
				"--output-format", "--json-schema", "--cwd", "--rules",
				"--system-prompt-override", "--compaction-mode",
				"--compaction-detail", "--allow", "--deny", "--agent", "--agents",
				"--tools", "--disallowed-tools", "--max-turns", "--permission-mode",
				"--sandbox", "--debug-file", "--leader-socket", "--worktree-ref",
				"--ref").
			with(optionalValue, "-w", "--worktree"),
		drop: opts{}.
			with(noValue, "-c", "--continue", "--fork-session", "--restore-code").
			with(oneValue, "--load", "-s", "--session-id", "-p", "--single",
				"--prompt-json", "--prompt-file").
			with(optionalValue, "-r", "--resume"),
	},
	// npm @letta-ai/letta-code 0.34.23 letta.js CLI_FLAG_CATALOG.
	"letta": {
		command:  "letta",
		names:    []string{"letta-code"},
		packages: []string{"/node_modules/@letta-ai/letta-code/letta"},
		refKinds: idRef,
		resume: func(ref string) []string {
			agent, isDefault := strings.CutPrefix(ref, "default:")
			switch {
			case !isDefault:
				return []string{"--conversation", ref}
			case agent == "":
				return nil
			}
			return []string{"--conversation", "default", "--agent", agent}
		},
		values: opts{}.
			with(oneValue, "-m", "--model", "-s", "--system", "--system-custom",
				"--embedding", "--personality", "--toolset", "--base-tools",
				"--backend", "--tools", "--allowedTools", "--disallowedTools",
				"--permission-mode", "--skills", "--skill-sources",
				"--reflection-trigger", "--reflection-step-count",
				"--output-format", "--input-format"),
		drop: opts{}.
			with(noValue, "--new", "--new-agent", "-r", "--resume", "-p", "--prompt",
				"--ephemeral", "--stateless").
			with(oneValue, "-C", "--conversation", "--conv", "-a", "--agent", "-n",
				"--name"),
	},
}
