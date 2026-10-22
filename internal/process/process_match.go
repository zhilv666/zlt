package process

import (
	"path"
	"runtime"
	"strings"
)

func commandMatchesTask(commandLine string, programPath string, args []string) bool {
	if !commandMatchesProgram(commandLine, programPath) {
		return false
	}
	commandArgs := extractCommandArgs(commandLine)
	if len(commandArgs) != len(args) {
		return false
	}

	for i := range args {
		if !sameCommandArg(commandArgs[i], args[i]) {
			return false
		}
	}
	return true
}

func commandMatchesProgram(commandLine string, programPath string) bool {
	if strings.TrimSpace(commandLine) == "" || strings.TrimSpace(programPath) == "" {
		return false
	}

	return sameExecutable(extractCommandPath(commandLine), programPath)
}

func extractCommandArgs(commandLine string) []string {
	fields := splitCommandLine(commandLine)
	if len(fields) <= 1 {
		return nil
	}
	return fields[1:]
}

func extractCommandPath(commandLine string) string {
	fields := splitCommandLine(commandLine)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func splitCommandLine(commandLine string) []string {
	line := strings.TrimSpace(commandLine)
	if line == "" {
		return nil
	}

	var parts []string
	var current strings.Builder
	inQuotes := false
	started := false

	for _, ch := range line {
		switch {
		case ch == '"':
			inQuotes = !inQuotes
			started = true
		case !inQuotes && (ch == ' ' || ch == '\t'):
			if started {
				parts = append(parts, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(ch)
			started = true
		}
	}

	if started {
		parts = append(parts, current.String())
	}
	return parts
}

func sameExecutable(left string, right string) bool {
	return left != "" && right != "" && normalizeProcessValue(left) == normalizeProcessValue(right)
}

func sameCommandArg(left string, right string) bool {
	return left == right
}

func normalizeProcessValue(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.Trim(normalized, `"`)
	if runtime.GOOS == "windows" {
		normalized = strings.ReplaceAll(normalized, `\`, `/`)
		normalized = strings.ToLower(normalized)
	}
	return path.Clean(normalized)
}

func sameWorkingDir(left string, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	leftNorm := normalizeProcessValue(left)
	rightNorm := normalizeProcessValue(right)
	if leftNorm == "" || rightNorm == "" {
		return false
	}
	return leftNorm == rightNorm
}
