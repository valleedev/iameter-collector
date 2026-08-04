package platform

import (
	"os"
	"path/filepath"
)

// Dirs is the set of filesystem locations IA METER uses. Only ConfigDir and
// DataDir are exposed as CLI flags / env overrides (--config-dir,
// --data-dir); CacheDir, LogDir and BinDir are derived defaults not
// explicitly listed in the spec's two example paths per OS, extended here
// because the CLI (doctor, daemon logs, install) needs somewhere concrete
// to put them.
type Dirs struct {
	ConfigDir string
	DataDir   string
	CacheDir  string
	LogDir    string
	BinDir    string
}

// DefaultDirs computes the per-OS default locations described in section 19
// of the spec, honoring XDG_* env vars on Linux.
func DefaultDirs() Dirs {
	home, _ := os.UserHomeDir()

	switch OS() {
	case "darwin":
		appSupport := filepath.Join(home, "Library", "Application Support", "IAMeter")
		return Dirs{
			ConfigDir: appSupport,
			DataDir:   filepath.Join(appSupport, "data"),
			CacheDir:  filepath.Join(home, "Library", "Caches", "IAMeter"),
			LogDir:    filepath.Join(home, "Library", "Logs", "IAMeter"),
			BinDir:    filepath.Join(appSupport, "bin"),
		}
	case "windows":
		base := os.Getenv("LOCALAPPDATA")
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		root := filepath.Join(base, "IAMeter")
		return Dirs{
			ConfigDir: root,
			DataDir:   filepath.Join(root, "data"),
			CacheDir:  filepath.Join(root, "cache"),
			LogDir:    filepath.Join(root, "logs"),
			BinDir:    filepath.Join(root, "bin"),
		}
	default: // linux and other unix
		configHome := os.Getenv("XDG_CONFIG_HOME")
		if configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		cacheHome := os.Getenv("XDG_CACHE_HOME")
		if cacheHome == "" {
			cacheHome = filepath.Join(home, ".cache")
		}
		return Dirs{
			ConfigDir: filepath.Join(configHome, "iameter"),
			DataDir:   filepath.Join(dataHome, "iameter"),
			CacheDir:  filepath.Join(cacheHome, "iameter"),
			LogDir:    filepath.Join(dataHome, "iameter", "logs"),
			BinDir:    filepath.Join(home, ".local", "bin"),
		}
	}
}

// BinaryName returns the platform-specific executable file name.
func BinaryName() string {
	if IsWindows() {
		return "iameter.exe"
	}
	return "iameter"
}
