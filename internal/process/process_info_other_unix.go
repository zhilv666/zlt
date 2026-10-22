//go:build unix && !linux && !darwin

package process

// Without a platform identity API, do not guess from ps output.
func readUnixProcessCommand(int) (string, []string, bool) { return "", nil, false }
func unixProcessMatchesWorkingDir(int, string) bool       { return false }
