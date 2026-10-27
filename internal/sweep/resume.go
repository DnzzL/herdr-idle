package sweep

import (
	"path/filepath"
	"strings"
)

// arity is how many values an option takes on the command line.
type arity int

const (
	noValue       arity = iota
	oneValue            // --flag value
	optionalValue       // --flag [value]: takes the next token unless it is an option
)

// recipe is how one agent kind resumes a session, mirroring herdr 0.9.5's
// restart planner (agent_resume.rs) but keeping the user's other flags.
type recipe struct {
	// command is the bare command typed at the prompt.
	command string
	// refKind is the herdr session ref kind the recipe takes: "id" or "path".
	refKind string
	// resume goes right after command; refToken stands for the session ref.
	resume []string
	// drop lists the options removed from the original argv first, so an
	// earlier resume, continue or fork cannot fight the new one.
	drop map[string]arity
}

const refToken = "{ref}"

// recipes is keyed by herdr agent kind.
var recipes = map[string]recipe{
	"claude": {
		command: "claude",
		refKind: "id",
		resume:  []string{"--resume", refToken},
		drop: map[string]arity{
			"--resume":       optionalValue,
			"-r":             optionalValue,
			"--continue":     noValue,
			"-c":             noValue,
			"--session-id":   oneValue,
			"--fork-session": noValue,
		},
	},
}

// resumeArgv builds the command that resumes session ref for an agent of
// kind started as argv: the recipe's command and resume tokens, then the
// original arguments minus the options the recipe drops. It reports false
// when the kind has no recipe or argv does not run that kind's command.
func resumeArgv(kind string, argv []string, ref string) ([]string, bool) {
	r, ok := recipes[kind]
	if !ok {
		return nil, false
	}
	// The command may be a path, or a script run by an interpreter
	// (node /usr/local/bin/claude …): flags start after it.
	start := -1
	for i, arg := range argv {
		if filepath.Base(arg) == r.command {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil, false
	}

	out := []string{r.command}
	for _, tok := range r.resume {
		out = append(out, strings.ReplaceAll(tok, refToken, ref))
	}
	args := argv[start:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			out = append(out, args[i:]...)
			break
		}
		name, _, inline := strings.Cut(arg, "=")
		n, drop := r.drop[name]
		if !drop {
			out = append(out, arg)
			continue
		}
		if inline {
			continue
		}
		hasNext := i+1 < len(args)
		switch {
		case n == oneValue && hasNext,
			n == optionalValue && hasNext && !strings.HasPrefix(args[i+1], "-"):
			i++
		}
	}
	return out, true
}

// shellJoin quotes argv for a POSIX shell prompt.
func shellJoin(argv []string) string {
	quoted := make([]string, len(argv))
	for i, arg := range argv {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
