package toolsafety

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const findingKindCommand = "command"

var curlPipeShellPattern = regexp.MustCompile(`(?i)\b(curl|wget)\b[^|]*\|\s*(?:sudo\s+)?(?:sh|bash|zsh)\b`)

// ClassifyShell returns lightweight risk findings for a shell command string.
func ClassifyShell(command string, policy Policy) []Finding {
	command = strings.TrimSpace(command)
	if command == "" {
		return []Finding{allowedCommandFinding(command, "empty shell command")}
	}

	var findings []Finding
	if hasSudo(command) {
		findings = append(findings, deniedCommandFinding(command, "sudo", "shell command invokes sudo"))
	}
	if hasRMRF(command) {
		findings = append(findings, deniedCommandFinding(command, "rm -rf", "shell command recursively force-removes files"))
	}
	if curlPipeShellPattern.MatchString(command) {
		findings = append(findings, deniedCommandFinding(command, "curl/wget | sh", "shell command pipes downloaded content into a shell"))
	}
	if hasGitCleanFDX(command) {
		findings = append(findings, deniedCommandFinding(command, "git clean -fdx", "shell command removes untracked and ignored git files"))
	}
	findings = append(findings, classifySensitivePermissionChanges(command, policy)...)
	findings = append(findings, classifySensitiveRedirections(command, policy)...)

	if len(findings) == 0 {
		return []Finding{allowedCommandFinding(command, "no risky shell patterns detected")}
	}
	return dedupeCommandFindings(findings)
}

func allowedCommandFinding(command string, reason string) Finding {
	return Finding{
		Kind:     findingKindCommand,
		Command:  command,
		Decision: DecisionAllowed,
		Severity: SeverityInfo,
		Reason:   reason,
	}
}

func deniedCommandFinding(command string, pattern string, reason string) Finding {
	return Finding{
		Kind:     findingKindCommand,
		Command:  command,
		Pattern:  pattern,
		Decision: DecisionDenied,
		Severity: SeverityDanger,
		Reason:   reason,
	}
}

func hasSudo(command string) bool {
	for _, token := range shellFields(command) {
		if filepath.Base(token) == "sudo" {
			return true
		}
	}
	return false
}

func hasRMRF(command string) bool {
	fields := shellFields(command)
	for i, field := range fields {
		if filepath.Base(field) != "rm" {
			continue
		}
		for _, arg := range fields[i+1:] {
			if isShellBoundary(arg) {
				break
			}
			if rmArgHasRecursiveForce(arg) {
				return true
			}
		}
	}
	return false
}

func rmArgHasRecursiveForce(arg string) bool {
	if arg == "-rf" || arg == "-fr" {
		return true
	}
	if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
		return strings.Contains(arg, "r") && strings.Contains(arg, "f")
	}
	return false
}

func hasGitCleanFDX(command string) bool {
	fields := shellFields(command)
	for i := 0; i+1 < len(fields); i++ {
		if filepath.Base(fields[i]) != "git" || fields[i+1] != "clean" {
			continue
		}
		var hasF, hasD, hasX bool
		for _, arg := range fields[i+2:] {
			if isShellBoundary(arg) {
				break
			}
			if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
				hasF = hasF || strings.Contains(arg, "f")
				hasD = hasD || strings.Contains(arg, "d")
				hasX = hasX || strings.Contains(arg, "x")
			}
			if hasF && hasD && hasX {
				return true
			}
		}
	}
	return false
}

func classifySensitivePermissionChanges(command string, policy Policy) []Finding {
	fields := shellFields(command)
	var findings []Finding
	for i, field := range fields {
		base := filepath.Base(field)
		if base != "chmod" && base != "chown" && base != "chgrp" {
			continue
		}
		for _, arg := range fields[i+1:] {
			if isShellBoundary(arg) {
				break
			}
			if strings.HasPrefix(arg, "-") || !looksLikePath(arg) {
				continue
			}
			pathFinding := ClassifyPath(OperationWrite, arg, policy)
			if pathFinding.Decision == DecisionDenied {
				findings = append(findings, Finding{
					Kind:         findingKindCommand,
					Operation:    OperationWrite,
					Command:      command,
					Input:        arg,
					Pattern:      base + " sensitive",
					Decision:     DecisionDenied,
					Severity:     SeverityDanger,
					Reason:       fmt.Sprintf("shell command changes permissions or ownership for denied path: %s", pathFinding.Reason),
					ResolvedPath: pathFinding.ResolvedPath,
				})
			}
		}
	}
	return findings
}

func classifySensitiveRedirections(command string, policy Policy) []Finding {
	fields := shellFields(command)
	var findings []Finding
	for i, field := range fields {
		path := ""
		if isRedirectOperator(field) && i+1 < len(fields) {
			path = fields[i+1]
		} else if redirectPath := inlineRedirectPath(field); redirectPath != "" {
			path = redirectPath
		}
		if path == "" {
			continue
		}
		pathFinding := ClassifyPath(OperationWrite, path, policy)
		if pathFinding.Decision == DecisionDenied {
			findings = append(findings, Finding{
				Kind:         findingKindCommand,
				Operation:    OperationWrite,
				Command:      command,
				Input:        path,
				Pattern:      "redirection sensitive",
				Decision:     DecisionDenied,
				Severity:     SeverityDanger,
				Reason:       fmt.Sprintf("shell redirection writes to denied path: %s", pathFinding.Reason),
				ResolvedPath: pathFinding.ResolvedPath,
			})
		}
	}
	return findings
}

func shellFields(command string) []string {
	replacer := strings.NewReplacer(
		"&&", " && ",
		"||", " || ",
		";", " ; ",
		"|", " | ",
		">>", " >> ",
		"2>", " 2> ",
		"1>", " 1> ",
		"&>", " &> ",
		">", " > ",
	)
	rawFields := strings.Fields(replacer.Replace(command))
	fields := make([]string, 0, len(rawFields))
	for _, field := range rawFields {
		field = cleanShellToken(field)
		if field != "" {
			fields = append(fields, field)
		}
	}
	return fields
}

func cleanShellToken(token string) string {
	token = strings.TrimSpace(token)
	token = strings.Trim(token, `"'`)
	token = strings.TrimRight(token, ",")
	return token
}

func isShellBoundary(token string) bool {
	switch token {
	case "&&", "||", ";", "|":
		return true
	default:
		return false
	}
}

func isRedirectOperator(token string) bool {
	switch token {
	case ">", ">>", "1>", "2>", "&>":
		return true
	default:
		return false
	}
}

func inlineRedirectPath(token string) string {
	for _, op := range []string{">>", "1>", "2>", "&>", ">"} {
		if strings.HasPrefix(token, op) && len(token) > len(op) {
			return strings.TrimSpace(token[len(op):])
		}
	}
	return ""
}

func looksLikePath(token string) bool {
	if token == "" {
		return false
	}
	if strings.HasPrefix(token, "/") || strings.HasPrefix(token, ".") || strings.HasPrefix(token, "~") {
		return true
	}
	return strings.Contains(token, "/")
}

func dedupeCommandFindings(findings []Finding) []Finding {
	seen := make(map[string]bool, len(findings))
	deduped := make([]Finding, 0, len(findings))
	for _, finding := range findings {
		key := finding.Pattern + "\x00" + finding.Input + "\x00" + finding.ResolvedPath
		if seen[key] {
			continue
		}
		seen[key] = true
		deduped = append(deduped, finding)
	}
	return deduped
}
