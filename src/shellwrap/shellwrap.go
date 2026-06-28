package shellwrap

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

type Result struct {
	Argv            []string
	Stripped        []Wrapper
	Changed         bool
	BlockedReason   string
	ParseError      string
	UnchangedReason string
}

type Wrapper struct {
	Name string
	Args []string
}

var durationPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?[smhd]?$`)
var niceAdjustmentPattern = regexp.MustCompile(`^[+-]?[0-9]+$`)
var compactNiceAdjustmentPattern = regexp.MustCompile(`^-[0-9]+$`)
var stdbufModePattern = regexp.MustCompile(`^(0|L|[1-9][0-9]*[KMGkmg]?)$`)

func Strip(argv []string) Result {
	original := cloneStrings(argv)
	current := cloneStrings(argv)
	var stripped []Wrapper

	for len(current) > 0 {
		name, ok := wrapperName(current[0])
		if !ok {
			result := Result{
				Argv:    cloneStrings(current),
				Changed: len(stripped) > 0,
			}
			if len(stripped) > 0 {
				result.Stripped = cloneWrappers(stripped)
			} else {
				result.UnchangedReason = "argv does not start with a recognized shell wrapper"
			}
			return result
		}

		commandStart, err := parseWrapper(name, current)
		if err != "" {
			return blocked(original, err)
		}
		stripped = append(stripped, Wrapper{
			Name: name,
			Args: cloneStrings(current[1:commandStart]),
		})
		current = cloneStrings(current[commandStart:])
	}

	return blocked(original, "shell wrapper parsing removed every argv element")
}

func Normalize(argv []string) Result {
	return Strip(argv)
}

func parseWrapper(name string, argv []string) (int, string) {
	switch name {
	case "timeout":
		return parseTimeout(argv)
	case "nice":
		return parseNice(argv)
	case "stdbuf":
		return parseStdbuf(argv)
	case "env":
		return parseEnv(argv)
	case "time":
		return parseTime(argv)
	case "nohup":
		return parseNohup(argv)
	default:
		return 0, fmt.Sprintf("%s: not a supported shell wrapper", name)
	}
}

func parseTimeout(argv []string) (int, string) {
	i := 1
	for i < len(argv) {
		arg := argv[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		if !isSafeWrapperToken(arg) {
			return 0, fmt.Sprintf("timeout: unsafe option %q", arg)
		}

		switch {
		case arg == "--preserve-status" || arg == "--foreground" || arg == "-v" || arg == "--verbose":
			i++
		case arg == "-s" || arg == "--signal":
			if i+1 >= len(argv) {
				return 0, fmt.Sprintf("timeout: missing value for %q", arg)
			}
			if !isSafeSignal(argv[i+1]) {
				return 0, fmt.Sprintf("timeout: unsafe signal value %q", argv[i+1])
			}
			i += 2
		case strings.HasPrefix(arg, "--signal="):
			if !isSafeSignal(strings.TrimPrefix(arg, "--signal=")) {
				return 0, fmt.Sprintf("timeout: unsafe signal option %q", arg)
			}
			i++
		case strings.HasPrefix(arg, "-s") && len(arg) > 2:
			if !isSafeSignal(arg[2:]) {
				return 0, fmt.Sprintf("timeout: unsafe signal option %q", arg)
			}
			i++
		case arg == "-k" || arg == "--kill-after":
			if i+1 >= len(argv) {
				return 0, fmt.Sprintf("timeout: missing value for %q", arg)
			}
			if !isDuration(argv[i+1]) {
				return 0, fmt.Sprintf("timeout: invalid kill-after duration %q", argv[i+1])
			}
			i += 2
		case strings.HasPrefix(arg, "--kill-after="):
			if !isDuration(strings.TrimPrefix(arg, "--kill-after=")) {
				return 0, fmt.Sprintf("timeout: invalid kill-after option %q", arg)
			}
			i++
		case strings.HasPrefix(arg, "-k") && len(arg) > 2:
			if !isDuration(arg[2:]) {
				return 0, fmt.Sprintf("timeout: invalid kill-after option %q", arg)
			}
			i++
		default:
			return 0, fmt.Sprintf("timeout: unsupported option %q", arg)
		}
	}

	if i >= len(argv) {
		return 0, "timeout: missing duration"
	}
	if !isDuration(argv[i]) {
		return 0, fmt.Sprintf("timeout: invalid duration %q", argv[i])
	}
	i++
	if i >= len(argv) {
		return 0, "timeout: missing command"
	}
	return i, ""
}

func parseNice(argv []string) (int, string) {
	i := 1
	sawAdjustment := false
	for i < len(argv) {
		arg := argv[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		if !isSafeWrapperToken(arg) {
			return 0, fmt.Sprintf("nice: unsafe option %q", arg)
		}

		switch {
		case arg == "-n" || arg == "--adjustment":
			if sawAdjustment {
				return 0, "nice: repeated adjustment option"
			}
			if i+1 >= len(argv) {
				return 0, fmt.Sprintf("nice: missing value for %q", arg)
			}
			if !isNiceAdjustment(argv[i+1]) {
				return 0, fmt.Sprintf("nice: invalid adjustment %q", argv[i+1])
			}
			sawAdjustment = true
			i += 2
		case strings.HasPrefix(arg, "--adjustment="):
			if sawAdjustment {
				return 0, "nice: repeated adjustment option"
			}
			if !isNiceAdjustment(strings.TrimPrefix(arg, "--adjustment=")) {
				return 0, fmt.Sprintf("nice: invalid adjustment option %q", arg)
			}
			sawAdjustment = true
			i++
		case strings.HasPrefix(arg, "-n") && len(arg) > 2:
			if sawAdjustment {
				return 0, "nice: repeated adjustment option"
			}
			if !isNiceAdjustment(arg[2:]) {
				return 0, fmt.Sprintf("nice: invalid adjustment option %q", arg)
			}
			sawAdjustment = true
			i++
		case compactNiceAdjustmentPattern.MatchString(arg):
			if sawAdjustment {
				return 0, "nice: repeated adjustment option"
			}
			sawAdjustment = true
			i++
		default:
			return 0, fmt.Sprintf("nice: unsupported option %q", arg)
		}
	}
	if i >= len(argv) {
		return 0, "nice: missing command"
	}
	return i, ""
}

func parseEnv(argv []string) (int, string) {
	i := 1
	allowDashCommand := false

	for i < len(argv) {
		arg := argv[i]
		if arg == "--" {
			i++
			allowDashCommand = true
			break
		}
		if isEnvAssignment(arg) {
			break
		}
		if !strings.HasPrefix(arg, "-") {
			break
		}
		if arg == "-" {
			return 0, "env: unsupported option \"-\""
		}
		if !isSafeWrapperToken(arg) {
			return 0, fmt.Sprintf("env: unsafe option %q", arg)
		}

		switch {
		case arg == "-i" || arg == "--ignore-environment":
			i++
		case arg == "-u" || arg == "--unset":
			if i+1 >= len(argv) {
				return 0, fmt.Sprintf("env: missing value for %q", arg)
			}
			if !isEnvName(argv[i+1]) {
				return 0, fmt.Sprintf("env: invalid unset name %q", argv[i+1])
			}
			i += 2
		case strings.HasPrefix(arg, "--unset="):
			if !isEnvName(strings.TrimPrefix(arg, "--unset=")) {
				return 0, fmt.Sprintf("env: invalid unset option %q", arg)
			}
			i++
		case strings.HasPrefix(arg, "-u") && len(arg) > 2:
			if !isEnvName(arg[2:]) {
				return 0, fmt.Sprintf("env: invalid unset option %q", arg)
			}
			i++
		default:
			return 0, fmt.Sprintf("env: unsupported option %q", arg)
		}
	}

	for i < len(argv) && isEnvAssignment(argv[i]) {
		if strings.ContainsRune(argv[i], '\x00') {
			return 0, "env: assignment contains NUL"
		}
		i++
	}
	if i < len(argv) && argv[i] == "--" {
		i++
		allowDashCommand = true
	}
	if i >= len(argv) {
		return 0, "env: missing command"
	}
	if strings.HasPrefix(argv[i], "-") && !allowDashCommand {
		return 0, fmt.Sprintf("env: command %q requires -- delimiter", argv[i])
	}
	return i, ""
}

func parseStdbuf(argv []string) (int, string) {
	i := 1
	sawOption := false
	for i < len(argv) {
		arg := argv[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		if !isSafeWrapperToken(arg) {
			return 0, fmt.Sprintf("stdbuf: unsafe option %q", arg)
		}

		switch {
		case arg == "-i" || arg == "-o" || arg == "-e" || arg == "--input" || arg == "--output" || arg == "--error":
			if i+1 >= len(argv) {
				return 0, fmt.Sprintf("stdbuf: missing value for %q", arg)
			}
			if !isStdbufMode(argv[i+1]) {
				return 0, fmt.Sprintf("stdbuf: invalid mode %q", argv[i+1])
			}
			sawOption = true
			i += 2
		case strings.HasPrefix(arg, "--input="):
			if !isStdbufMode(strings.TrimPrefix(arg, "--input=")) {
				return 0, fmt.Sprintf("stdbuf: invalid input option %q", arg)
			}
			sawOption = true
			i++
		case strings.HasPrefix(arg, "--output="):
			if !isStdbufMode(strings.TrimPrefix(arg, "--output=")) {
				return 0, fmt.Sprintf("stdbuf: invalid output option %q", arg)
			}
			sawOption = true
			i++
		case strings.HasPrefix(arg, "--error="):
			if !isStdbufMode(strings.TrimPrefix(arg, "--error=")) {
				return 0, fmt.Sprintf("stdbuf: invalid error option %q", arg)
			}
			sawOption = true
			i++
		case len(arg) > 2 && (arg[1] == 'i' || arg[1] == 'o' || arg[1] == 'e'):
			if !isStdbufMode(arg[2:]) {
				return 0, fmt.Sprintf("stdbuf: invalid option %q", arg)
			}
			sawOption = true
			i++
		default:
			return 0, fmt.Sprintf("stdbuf: unsupported option %q", arg)
		}
	}

	if !sawOption {
		return 0, "stdbuf: missing io/e option"
	}
	if i >= len(argv) {
		return 0, "stdbuf: missing command"
	}
	return i, ""
}

func parseTime(argv []string) (int, string) {
	i := 1
	for i < len(argv) {
		arg := argv[i]
		if arg == "--" {
			i++
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		if !isSafeWrapperToken(arg) {
			return 0, fmt.Sprintf("time: unsafe option %q", arg)
		}

		switch arg {
		case "-p", "--portability", "-v", "--verbose", "--quiet":
			i++
		default:
			return 0, fmt.Sprintf("time: unsupported option %q", arg)
		}
	}
	if i >= len(argv) {
		return 0, "time: missing command"
	}
	return i, ""
}

func parseNohup(argv []string) (int, string) {
	i := 1
	allowDashCommand := false
	if i >= len(argv) {
		return 0, "nohup: missing command"
	}
	if argv[i] == "--" {
		i++
		allowDashCommand = true
	}
	if i >= len(argv) {
		return 0, "nohup: missing command"
	}
	if strings.HasPrefix(argv[i], "-") && !allowDashCommand {
		return 0, fmt.Sprintf("nohup: command %q requires -- delimiter", argv[i])
	}
	return i, ""
}

func blocked(original []string, reason string) Result {
	return Result{
		Argv:          cloneStrings(original),
		BlockedReason: reason,
		ParseError:    reason,
	}
}

func wrapperName(arg string) (string, bool) {
	switch arg {
	case "timeout", "nice", "stdbuf", "env", "time", "nohup":
		return arg, true
	default:
		return "", false
	}
}

func cloneStrings(values []string) []string {
	if values == nil {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}

func cloneWrappers(wrappers []Wrapper) []Wrapper {
	out := make([]Wrapper, len(wrappers))
	for i, wrapper := range wrappers {
		out[i] = Wrapper{
			Name: wrapper.Name,
			Args: cloneStrings(wrapper.Args),
		}
	}
	return out
}

func isDuration(value string) bool {
	return durationPattern.MatchString(value)
}

func isNiceAdjustment(value string) bool {
	return niceAdjustmentPattern.MatchString(value)
}

func isStdbufMode(value string) bool {
	return stdbufModePattern.MatchString(value)
}

func isSafeSignal(value string) bool {
	if value == "" || !isSafeWrapperToken(value) {
		return false
	}
	for _, r := range value {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func isSafeWrapperToken(value string) bool {
	if value == "" || strings.ContainsRune(value, '\x00') {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '-', '_', '.', '/', '=', '+':
			continue
		default:
			return false
		}
	}
	return true
}

func isEnvAssignment(value string) bool {
	if strings.ContainsRune(value, '\x00') {
		return false
	}
	idx := strings.IndexByte(value, '=')
	if idx <= 0 {
		return false
	}
	return isEnvName(value[:idx])
}

func isEnvName(value string) bool {
	if value == "" {
		return false
	}
	for i, r := range value {
		if i == 0 {
			if r != '_' && !unicode.IsLetter(r) {
				return false
			}
			continue
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
