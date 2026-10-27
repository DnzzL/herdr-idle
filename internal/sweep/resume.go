package sweep

import (
	"path/filepath"
	"slices"
	"strings"
)

// arity is how many values an option takes on the command line.
type arity int

const (
	noValue       arity = iota
	oneValue            // --flag value
	optionalValue       // --flag [value]: takes the next token unless it is an option
	manyValues          // --flag v1 v2…: takes tokens up to the next option
)

// recipe is how one agent kind resumes a session, mirroring herdr 0.9.5's
// restart planner (agent_resume.rs) but keeping the user's other flags.
type recipe struct {
	// command is the bare command typed at the prompt.
	command string
	// names are the other basenames the command runs under, and packages
	// path fragments of the script a runtime (node, bun) runs for it, as
	// herdr's detector knows them (src/detect/mod.rs).
	names    []string
	packages []string
	// refKinds are the herdr session ref kinds the recipe takes.
	refKinds []string
	// resume returns the tokens put right after command for session ref,
	// or nil when ref cannot be resumed.
	resume func(ref string) []string
	// values lists the options that take a value, kept with it. Any other
	// bare word is a positional argument (a prompt, a subcommand) and is
	// dropped, so the resumed session does not replay it. An unknown
	// option is kept on its own: if it took a value, that value is lost.
	values map[string]arity
	// drop lists the options removed from the original argv, with their
	// value, so an earlier resume, continue or fork cannot fight the new one.
	drop map[string]arity
}

// resumeArgv builds the command that resumes session ref for an agent of
// kind started as argv: the recipe's command and resume tokens, then the
// original options minus the ones the recipe drops. It reports false when
// the kind has no recipe, ref cannot be resumed, or argv does not run that
// kind's command.
func resumeArgv(kind string, argv []string, ref string) ([]string, bool) {
	r, ok := recipes[kind]
	if !ok {
		return nil, false
	}
	resume := r.resume(ref)
	if resume == nil {
		return nil, false
	}
	// The command may be a path, or a script run by an interpreter
	// (node /usr/local/bin/claude …): options start after it.
	start := slices.IndexFunc(argv, r.runs) + 1
	if start == 0 {
		return nil, false
	}

	out := append([]string{r.command}, resume...)
	args := argv[start:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break // the rest is positional
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			continue // positional
		}
		name, _, inline := strings.Cut(arg, "=")
		n, drop := r.drop[name]
		if !drop {
			out = append(out, arg)
			n = r.values[name]
		}
		if inline {
			continue
		}
		for nth := 1; i+1 < len(args) && takes(n, nth, args[i+1]); nth++ {
			i++
			if !drop {
				out = append(out, args[i])
			}
		}
	}
	return out, true
}

// takes reports whether an option of arity n takes tok as its nth value.
func takes(n arity, nth int, tok string) bool {
	switch n {
	case oneValue:
		return nth == 1
	case optionalValue:
		return nth == 1 && !strings.HasPrefix(tok, "-")
	case manyValues:
		return !strings.HasPrefix(tok, "-")
	}
	return false
}

// runs reports whether arg is the recipe's command, by basename or as a
// known script path.
func (r recipe) runs(arg string) bool {
	base := strings.ToLower(filepath.Base(arg))
	for _, ext := range []string{".js", ".mjs", ".cjs"} {
		base = strings.TrimSuffix(base, ext)
	}
	if base == r.command || slices.Contains(r.names, base) {
		return true
	}
	lower := strings.ToLower(arg)
	return slices.ContainsFunc(r.packages, func(p string) bool { return strings.Contains(lower, p) })
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
