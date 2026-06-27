package shellhooks

import (
	"fmt"
	"strings"
)

func ResolveArgv(spec Spec) ([]string, error) {
	if len(spec.Argv) > 0 {
		argv := append([]string(nil), spec.Argv...)
		if strings.TrimSpace(argv[0]) == "" {
			return nil, fmt.Errorf("argv[0] is empty")
		}
		return argv, nil
	}
	return SplitCommand(spec.Command)
}

// SplitCommand tokenizes a command string into argv without invoking a shell.
// It supports whitespace, single quotes, double quotes, and backslash escaping;
// it does not expand variables, globs, command substitutions, or redirections.
func SplitCommand(command string) ([]string, error) {
	var argv []string
	var b strings.Builder
	var quote rune
	escaped := false
	started := false

	flush := func() {
		argv = append(argv, b.String())
		b.Reset()
		started = false
	}

	for _, r := range command {
		if escaped {
			b.WriteRune(r)
			started = true
			escaped = false
			continue
		}

		if quote != '\'' && r == '\\' {
			escaped = true
			started = true
			continue
		}

		if quote != 0 {
			if r == quote {
				quote = 0
				started = true
				continue
			}
			b.WriteRune(r)
			started = true
			continue
		}

		switch r {
		case '\'', '"':
			quote = r
			started = true
		case ' ', '\t', '\n', '\r':
			if started {
				flush()
			}
		default:
			b.WriteRune(r)
			started = true
		}
	}

	if escaped {
		return nil, fmt.Errorf("command has trailing escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("command has unterminated quote")
	}
	if started {
		flush()
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("command is empty")
	}
	return argv, nil
}
