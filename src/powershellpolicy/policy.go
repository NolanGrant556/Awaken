package powershellpolicy

import (
	"strconv"
	"strings"
)

type Behavior string

const (
	BehaviorPassthrough Behavior = "passthrough"
	BehaviorAsk         Behavior = "ask"
	BehaviorAllow       Behavior = "allow"
)

type Decision struct {
	Behavior Behavior
	Message  string
}

type CommandResult struct {
	IsError bool
	Message string
}

type parsedCommand struct {
	rawName   string
	name      string
	args      []string
	statement int
}

type tokenKind int

const (
	tokenWord tokenKind = iota
	tokenPipe
	tokenStatement
)

type token struct {
	kind tokenKind
	text string
}

var (
	powerShellExecutables = map[string]struct{}{
		"pwsh":       {},
		"powershell": {},
	}
	downloaders = map[string]struct{}{
		"invoke-webrequest":  {},
		"iwr":                {},
		"invoke-restmethod":  {},
		"irm":                {},
		"new-object":         {},
		"start-bitstransfer": {},
	}
)

func Analyze(command string) Decision {
	tokens, ok := lex(command)
	if !ok {
		return ask("Could not parse command for security analysis")
	}
	commands := parseCommands(tokens)

	if result := checkDownloadCradles(commands); result.Behavior == BehaviorAsk {
		return result
	}
	if result := checkInvokeExpression(commands); result.Behavior == BehaviorAsk {
		return result
	}
	if result := checkEncodedCommand(commands); result.Behavior == BehaviorAsk {
		return result
	}
	if result := checkNestedPowerShell(commands); result.Behavior == BehaviorAsk {
		return result
	}
	if result := checkDownloadUtilities(commands); result.Behavior == BehaviorAsk {
		return result
	}
	if result := checkAddType(commands); result.Behavior == BehaviorAsk {
		return result
	}
	if result := checkComObject(commands); result.Behavior == BehaviorAsk {
		return result
	}
	if result := checkStartProcess(commands); result.Behavior == BehaviorAsk {
		return result
	}

	return Decision{Behavior: BehaviorPassthrough}
}

func InterpretCommandResult(command string, exitCode int, stdout string, stderr string) CommandResult {
	tokens, ok := lex(command)
	if !ok {
		return defaultCommandResult(exitCode)
	}

	commands := parseCommands(tokens)
	base := ""
	if len(commands) > 0 {
		base = commandBase(commands[len(commands)-1].rawName)
	}

	switch base {
	case "grep", "rg", "findstr":
		if exitCode == 1 {
			return CommandResult{Message: "No matches found"}
		}
		if exitCode >= 2 {
			return CommandResult{IsError: true, Message: exitCodeMessage(exitCode)}
		}
		return CommandResult{}
	case "robocopy":
		if exitCode >= 8 {
			return CommandResult{IsError: true, Message: exitCodeMessage(exitCode)}
		}
		if exitCode == 0 {
			return CommandResult{Message: "No files copied (already in sync)"}
		}
		if exitCode&1 == 1 {
			return CommandResult{Message: "Files copied successfully"}
		}
		return CommandResult{Message: "Robocopy completed (no errors)"}
	default:
		return defaultCommandResult(exitCode)
	}
}

func checkInvokeExpression(commands []parsedCommand) Decision {
	for _, cmd := range commands {
		if isIEX(cmd.name) {
			return ask("Command uses Invoke-Expression which can execute arbitrary code")
		}
	}
	return passthrough()
}

func checkEncodedCommand(commands []parsedCommand) Decision {
	for _, cmd := range commands {
		if !isPowerShellExecutable(cmd.rawName) {
			continue
		}
		for _, arg := range cmd.args {
			if paramMatches(arg, "encodedcommand", "e") {
				return ask("Command uses encoded parameters which obscure intent")
			}
		}
	}
	return passthrough()
}

func checkNestedPowerShell(commands []parsedCommand) Decision {
	for _, cmd := range commands {
		if isPowerShellExecutable(cmd.rawName) {
			return ask("Command spawns a nested PowerShell process which cannot be validated")
		}
	}
	return passthrough()
}

func checkDownloadCradles(commands []parsedCommand) Decision {
	downloaderByStatement := map[int]bool{}
	iexByStatement := map[int]bool{}
	hasDownloader := false
	hasIEX := false

	for _, cmd := range commands {
		if _, ok := downloaders[cmd.name]; ok {
			downloaderByStatement[cmd.statement] = true
			hasDownloader = true
		}
		if isIEX(cmd.name) {
			iexByStatement[cmd.statement] = true
			hasIEX = true
		}
	}

	for statement := range downloaderByStatement {
		if iexByStatement[statement] {
			return ask("Command downloads and executes remote code")
		}
	}
	if hasDownloader && hasIEX {
		return ask("Command downloads and executes remote code")
	}

	return passthrough()
}

func checkDownloadUtilities(commands []parsedCommand) Decision {
	for _, cmd := range commands {
		switch cmd.name {
		case "start-bitstransfer":
			return ask("Command downloads files via BITS transfer")
		case "certutil":
			if hasParam(cmd.args, "urlcache", "urlcache") {
				return ask("Command uses certutil to download from a URL")
			}
		case "bitsadmin":
			if hasParam(cmd.args, "transfer", "transfer") {
				return ask("Command downloads files via BITS transfer")
			}
		}
	}
	return passthrough()
}

func checkAddType(commands []parsedCommand) Decision {
	for _, cmd := range commands {
		if cmd.name == "add-type" {
			return ask("Command compiles and loads .NET code")
		}
	}
	return passthrough()
}

func checkComObject(commands []parsedCommand) Decision {
	for _, cmd := range commands {
		if cmd.name != "new-object" {
			continue
		}
		for _, arg := range cmd.args {
			if paramMatches(arg, "comobject", "com") {
				return ask("Command instantiates a COM object which may have execution capabilities")
			}
		}
	}
	return passthrough()
}

func checkStartProcess(commands []parsedCommand) Decision {
	for _, cmd := range commands {
		if cmd.name != "start-process" && cmd.name != "saps" && cmd.name != "start" {
			continue
		}

		hasVerb := false
		for _, arg := range cmd.args {
			if name, value, ok := splitParam(arg); ok && paramNameMatches(name, "verb", "v") {
				hasVerb = true
				if normalizeValue(value) == "runas" {
					return ask("Command requests elevated privileges")
				}
			}
		}
		if hasVerb {
			for _, arg := range cmd.args {
				if normalizeValue(arg) == "runas" {
					return ask("Command requests elevated privileges")
				}
			}
		}

		for _, arg := range cmd.args {
			if _, value, ok := splitParam(arg); ok && value != "" && isPowerShellExecutable(value) {
				return ask("Start-Process launches a nested PowerShell process which cannot be validated")
			}
			if isPowerShellExecutable(arg) {
				return ask("Start-Process launches a nested PowerShell process which cannot be validated")
			}
		}
	}
	return passthrough()
}

func parseCommands(tokens []token) []parsedCommand {
	var commands []parsedCommand
	var segment []token
	statement := 0

	flush := func() {
		if cmd, ok := commandFromSegment(segment, statement); ok {
			commands = append(commands, cmd)
		}
		segment = nil
	}

	for _, tok := range tokens {
		switch tok.kind {
		case tokenPipe:
			flush()
		case tokenStatement:
			flush()
			statement++
		default:
			segment = append(segment, tok)
		}
	}
	flush()

	return commands
}

func commandFromSegment(segment []token, statement int) (parsedCommand, bool) {
	words := make([]string, 0, len(segment))
	for _, tok := range segment {
		if tok.kind == tokenWord && tok.text != "" {
			words = append(words, tok.text)
		}
	}

	i := 0
	for i < len(words) && isCallOperator(words[i]) {
		i++
	}
	if i+2 <= len(words) && strings.HasPrefix(strings.TrimSpace(words[i]), "$") && words[i+1] == "=" {
		i += 2
		for i < len(words) && isCallOperator(words[i]) {
			i++
		}
	}
	if i >= len(words) {
		return parsedCommand{}, false
	}

	rawName := words[i]
	name := commandBase(rawName)
	if name == "" {
		return parsedCommand{}, false
	}

	return parsedCommand{
		rawName:   rawName,
		name:      name,
		args:      words[i+1:],
		statement: statement,
	}, true
}

func lex(command string) ([]token, bool) {
	var tokens []token
	var b strings.Builder
	quote := rune(0)

	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens = append(tokens, token{kind: tokenWord, text: b.String()})
		b.Reset()
	}

	runes := []rune(command)
	for i := 0; i < len(runes); i++ {
		r := runes[i]

		if r == '`' && i+1 < len(runes) {
			i++
			b.WriteRune(runes[i])
			continue
		}

		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			b.WriteRune(r)
			continue
		}

		switch {
		case r == '\'' || r == '"':
			quote = r
		case r == '\r' || r == '\n' || r == ';':
			flush()
			tokens = append(tokens, token{kind: tokenStatement})
		case r == '|':
			flush()
			if i+1 < len(runes) && runes[i+1] == '|' {
				i++
				tokens = append(tokens, token{kind: tokenStatement})
			} else {
				tokens = append(tokens, token{kind: tokenPipe})
			}
		case r == '&':
			flush()
			if i+1 < len(runes) && runes[i+1] == '&' {
				i++
				tokens = append(tokens, token{kind: tokenStatement})
			} else {
				tokens = append(tokens, token{kind: tokenWord, text: "&"})
			}
		case r == '=':
			flush()
			tokens = append(tokens, token{kind: tokenWord, text: "="})
		case r == '(' || r == ')' || r == '{' || r == '}':
			flush()
			tokens = append(tokens, token{kind: tokenStatement})
		case isSpace(r):
			flush()
		default:
			b.WriteRune(r)
		}
	}

	if quote != 0 {
		return tokens, false
	}
	flush()
	return tokens, true
}

func hasParam(args []string, fullName string, minName string) bool {
	for _, arg := range args {
		if paramMatches(arg, fullName, minName) {
			return true
		}
	}
	return false
}

func paramMatches(arg string, fullName string, minName string) bool {
	name, _, ok := splitParam(arg)
	return ok && paramNameMatches(name, fullName, minName)
}

func paramNameMatches(name string, fullName string, minName string) bool {
	name = strings.ToLower(name)
	fullName = strings.ToLower(strings.TrimLeft(fullName, "-/"))
	minName = strings.ToLower(strings.TrimLeft(minName, "-/"))

	return len(name) >= len(minName) && len(name) <= len(fullName) && strings.HasPrefix(fullName, name)
}

func splitParam(arg string) (string, string, bool) {
	clean := strings.TrimSpace(stripOuterQuotes(strings.ReplaceAll(arg, "`", "")))
	if clean == "" {
		return "", "", false
	}

	runes := []rune(clean)
	if !isParamPrefix(runes[0]) {
		return "", "", false
	}

	body := string(runes[1:])
	if body == "" {
		return "", "", false
	}

	if idx := strings.IndexRune(body, ':'); idx >= 0 {
		return strings.ToLower(body[:idx]), body[idx+1:], true
	}
	return strings.ToLower(body), "", true
}

func isParamPrefix(r rune) bool {
	switch r {
	case '-', '/', '\u2013', '\u2014', '\u2015', '\u2212':
		return true
	default:
		return false
	}
}

func isPowerShellExecutable(raw string) bool {
	_, ok := powerShellExecutables[commandBase(raw)]
	return ok
}

func commandBase(raw string) string {
	s := normalizeValue(raw)
	s = strings.Trim(s, " \t\r\n(){}")
	if idx := strings.LastIndexAny(s, `/\`); idx >= 0 {
		s = s[idx+1:]
	}
	s = strings.TrimSuffix(strings.ToLower(s), ".exe")
	return s
}

func normalizeValue(raw string) string {
	s := strings.ReplaceAll(raw, "`", "")
	s = stripOuterQuotes(strings.TrimSpace(s))
	return strings.ToLower(strings.TrimSpace(s))
}

func stripOuterQuotes(raw string) string {
	s := strings.TrimSpace(raw)
	for len(s) >= 2 {
		first := s[0]
		last := s[len(s)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			s = strings.TrimSpace(s[1 : len(s)-1])
			continue
		}
		break
	}
	return s
}

func isIEX(name string) bool {
	return name == "invoke-expression" || name == "iex"
}

func isCallOperator(word string) bool {
	return word == "&" || word == "."
}

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\f' || r == '\v'
}

func defaultCommandResult(exitCode int) CommandResult {
	if exitCode == 0 {
		return CommandResult{}
	}
	return CommandResult{IsError: true, Message: exitCodeMessage(exitCode)}
}

func exitCodeMessage(exitCode int) string {
	return "Command failed with exit code " + strconv.Itoa(exitCode)
}

func ask(message string) Decision {
	return Decision{Behavior: BehaviorAsk, Message: message}
}

func passthrough() Decision {
	return Decision{Behavior: BehaviorPassthrough}
}
