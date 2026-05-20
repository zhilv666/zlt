package process

import (
	"path"
	"path/filepath"
	"strings"
)

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

func extractCommandPath(commandLine string) string {
	line := strings.TrimSpace(commandLine)
	if line == "" {
		return ""
	}

	if strings.HasPrefix(line, `"`) {
		end := strings.Index(line[1:], `"`)
		if end >= 0 {
			return line[1 : end+1]
		}
	}

	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

func sameExecutable(left string, right string) bool {
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
