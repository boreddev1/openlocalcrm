package main

import (
	"os"
	"path/filepath"
	"runtime"
)

// ResolveProjectBaseDir determines the directory where configuration and docker files are stored.
func ResolveProjectBaseDir(customDir string) string {
	if customDir != "" {
		return customDir
	}

	if envDir := os.Getenv("OPENLOCALCRM_DIR"); envDir != "" {
		return envDir
	}

	// 1. Current working directory has docker-compose.yml or .env
	if _, err := os.Stat("docker-compose.yml"); err == nil {
		return "."
	}
	if _, err := os.Stat(".env"); err == nil {
		return "."
	}

	// 2. Executable directory or parent directory
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if _, err := os.Stat(filepath.Join(exeDir, "docker-compose.yml")); err == nil {
			return exeDir
		}
		parentDir := filepath.Dir(exeDir)
		if _, err := os.Stat(filepath.Join(parentDir, "docker-compose.yml")); err == nil {
			return parentDir
		}

		// Windows standard candidates
		if runtime.GOOS == "windows" {
			candidates := []string{
				"C:\\openlocalcrm",
				"C:\\mavalio",
				filepath.Join(os.Getenv("LOCALAPPDATA"), "openlocalcrm"),
				filepath.Join(os.Getenv("LOCALAPPDATA"), "mavalio"),
				filepath.Join(os.Getenv("ProgramFiles"), "OpenLocalCRM"),
				filepath.Join(os.Getenv("ProgramFiles"), "mavalio CRM"),
			}
			for _, c := range candidates {
				if c != "" {
					if _, err := os.Stat(filepath.Join(c, "docker-compose.yml")); err == nil {
						return c
					}
					if _, err := os.Stat(filepath.Join(c, ".env")); err == nil {
						return c
					}
				}
			}
			return exeDir
		}

		// Unix standard candidates: ~/.openlocalcrm
		if home, hErr := os.UserHomeDir(); hErr == nil {
			userDir := filepath.Join(home, ".openlocalcrm")
			if _, err := os.Stat(filepath.Join(userDir, "docker-compose.yml")); err == nil {
				return userDir
			}
			if _, err := os.Stat(filepath.Join(userDir, ".env")); err == nil {
				return userDir
			}
		}

		// If running from system PATH (/usr/local/bin, /usr/bin, etc.), default project dir to ~/.openlocalcrm
		if exeDir == "/usr/local/bin" || exeDir == "/usr/bin" || exeDir == "/bin" {
			if home, hErr := os.UserHomeDir(); hErr == nil {
				userDir := filepath.Join(home, ".openlocalcrm")
				_ = os.MkdirAll(userDir, 0755)
				return userDir
			}
		}

		return exeDir
	}

	return "."
}
