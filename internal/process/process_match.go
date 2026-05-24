package process

import (
	"path"
	"path/filepath"
	"strings"
)

func commandMatchesTask(commandLine string, programPath string, args []string) bool {
	if !commandMatchesProgram(commandLine, programPath) {
		return false
	}
	if len(args) == 0 {
		return true
	}

	commandArgs := extractCommandArgs(commandLine)
	if len(commandArgs) < len(args) {
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

	commandPath := extractCommandPath(commandLine)
	if commandPath != "" && sameExecutable(commandPath, programPath) {
		return true
	}

	return strings.Contains(normalizeProcessValue(commandLine), normalizeProcessValue(programPath))
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

	for _, ch := range line {
		switch {
		case ch == '"':
			inQuotes = !inQuotes
		case !inQuotes && (ch == ' ' || ch == '\t'):
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}
	return parts
}

func sameExecutable(left string, right string) bool {
	leftNorm := normalizeProcessValue(left)
	rightNorm := normalizeProcessValue(right)
	if leftNorm == rightNorm {
		return true
	}

	return strings.EqualFold(filepath.Base(leftNorm), filepath.Base(rightNorm))
}

func sameCommandArg(left string, right string) bool {
	leftNorm := normalizeProcessValue(left)
	rightNorm := normalizeProcessValue(right)
	if leftNorm == rightNorm {
		return true
	}

	return strings.EqualFold(filepath.Base(leftNorm), filepath.Base(rightNorm))
}

func normalizeProcessValue(value string) string {
	normalized := strings.TrimSpace(value)
	normalized = strings.Trim(normalized, `"`)
	normalized = strings.ReplaceAll(normalized, `\`, `/`)
	normalized = path.Clean(normalized)
	return strings.ToLower(normalized)
}

func sameWorkingDir(left string, right string) bool {
	leftNorm := normalizeProcessValue(left)
	rightNorm := normalizeProcessValue(right)
	if leftNorm == "" || rightNorm == "" {
		return false
	}
	return leftNorm == rightNorm
}
