//go:build !unix && !windows

package auth

// secureKeyFile is a no-op fallback for platforms without a dedicated
// implementation. Key file permissions are left to the os.WriteFile 0600
// mode hint; such platforms are not current deployment targets.
func secureKeyFile(path string) error { return nil }
