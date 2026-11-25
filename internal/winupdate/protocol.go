package winupdate

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	ApplyCommand    = "--update-install"
	VerifyCommand   = "--update-verify"
	CleanupCommand  = "--update-cleanup"
	RestartExit     = 76
	VersionFile     = "runtime-version"
	RuntimeLockFile = ".runtime-lock"
	UpdateLockFile  = ".update-lock"
)

// ErrRestart is returned after the workspace has closed and saved preferences.
var ErrRestart = errors.New("restart through the Windows launcher")

var versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

// ReleaseVersion reads the version recorded inside the runtime itself. For an
// update, this file is authenticated with the rest of the archive by MyGo.
func ReleaseVersion(runtime fs.FS) (string, error) {
	f, err := runtime.Open(VersionFile)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 65))
	if err != nil {
		return "", err
	}
	version := strings.TrimSuffix(string(b), "\n")
	if len(version) > 32 || !versionPattern.MatchString(version) {
		return "", errors.New("invalid Windows runtime version")
	}
	return version, nil
}

// RunningLauncher locates the launcher of the currently selected runtime.
// Development binaries and binaries in an older runtime cannot install an update.
func RunningLauncher() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	root := filepath.Dir(filepath.Dir(exe))
	store, err := os.OpenRoot(root)
	if err != nil {
		return "", err
	}
	defer store.Close()
	active, err := Current(store)
	if err != nil {
		return "", err
	}
	selected, err := os.Stat(active)
	if err != nil {
		return "", err
	}
	running, err := os.Stat(exe)
	if err != nil || !os.SameFile(running, selected) {
		return "", errors.New("application is not running from the active Windows runtime")
	}
	launcher, err := store.Lstat(appExecutable)
	if err != nil {
		return "", err
	}
	if !launcher.Mode().IsRegular() {
		return "", errors.New("invalid Windows launcher")
	}
	return filepath.Join(root, appExecutable), nil
}
