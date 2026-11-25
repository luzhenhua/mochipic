//go:build windows && amd64

package releaseqa

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"imageflow/internal/testkit"
	"imageflow/internal/winupdate"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const installRegistry = `Software\Microsoft\Windows\CurrentVersion\Uninstall\MochiPic`

var registryViews = []uint32{registry.WOW64_32KEY, registry.WOW64_64KEY}

type installerAsset struct {
	Name, Version, SHA256 string
}

func TestInstalledNsisLifecycle(t *testing.T) {
	root := os.Getenv("MOCHIPIC_WINDOWS_INSTALL_QA")
	if root == "" {
		t.Skip("use the clean Windows installation QA bundle")
	}
	var kit struct{ Previous, Candidate installerAsset }
	if err := json.Unmarshal(readWindowsFixture(t, filepath.Join(root, "installers.json")), &kit); err != nil {
		t.Fatal(err)
	}
	previous := checkedSetup(t, root, kit.Previous)
	candidate := checkedSetup(t, root, kit.Candidate)

	local, err := windows.KnownFolderPath(windows.FOLDERID_LocalAppData, 0)
	if err != nil {
		t.Fatal(err)
	}
	programs, err := windows.KnownFolderPath(windows.FOLDERID_Programs, 0)
	if err != nil {
		t.Fatal(err)
	}
	desktop, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0)
	if err != nil {
		t.Fatal(err)
	}
	settings, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	defaultInstall := filepath.Join(local, "Programs", "MochiPic")
	install := filepath.Join(t.TempDir(), "自选 安装目录")
	shortcut := filepath.Join(programs, "MochiPic.lnk")
	desktopLinks := []string{filepath.Join(desktop, "MochiPic.lnk"), filepath.Join(desktop, "麻薯图图.lnk")}
	preferencesDir := filepath.Join(settings, "ImageFlow")
	for _, name := range append(desktopLinks, defaultInstall, shortcut, preferencesDir) {
		assertAbsent(t, name)
	}
	checkInstalledRegistry(t, "", install)
	version := windows.RtlGetVersion()
	t.Logf("Windows %d.%d build %d, clean per-user install, elevated=%t", version.MajorVersion, version.MinorVersion,
		version.BuildNumber, windows.GetCurrentProcessToken().IsElevated())

	if output, err := runInstaller(t, previous, "/D="+install); err != nil {
		t.Fatalf("install %s: %v\n%s", kit.Previous.Version, err, output)
	}
	oldRuntime := checkInstalledRuntime(t, install, shortcut, kit.Previous.Version)
	work := filepath.Join(t.TempDir(), "图片 验收")
	if err := os.Mkdir(work, 0o700); err != nil {
		t.Fatal(err)
	}
	data := filepath.Join(root, "internal", "imaging", "testdata")
	input := filepath.Join(work, "原图 测试.jxl")
	sample := readWindowsFixture(t, filepath.Join(data, "still.jxl"))
	if err := os.WriteFile(input, sample, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(oldRuntime, "lib", "vips-modules-8.18", "vips-jxl.dll")); err == nil {
		// These released installers put the plugin outside libvips' search path.
		// Reproduce the actual initialization failure before testing upgrade.
		probe := exec.CommandContext(t.Context(), filepath.Join(oldRuntime, "MochiPic.exe"), "--verify-runtime")
		probe.Dir = work
		probe.Env = append(os.Environ(), "VIPSHOME=", "FONTCONFIG_FILE=", "MAGICK_CONFIGURE_PATH=")
		if output, err := probe.CombinedOutput(); err == nil || !strings.Contains(string(output), "JPEG XL") {
			t.Fatalf("expected the previous runtime to report the missing JPEG XL plugin: %v\n%s", err, output)
		}
		t.Log("previous installer reproduced the missing JPEG XL plugin at startup")
	} else {
		verifyInstalledWindow(t, install, work, filepath.Base(input))
	}

	if err := os.Mkdir(preferencesDir, 0o700); err != nil {
		t.Fatal(err)
	}
	fontSource := filepath.Join(oldRuntime, "etc", "fonts", "旧版 原图.jxl")
	exported := filepath.Join(install, "既有 导出.jxl")
	preferences := filepath.Join(preferencesDir, "preferences.json")
	protected := map[string][]byte{
		fontSource:  sample,
		exported:    sample,
		preferences: []byte(`{"Version":1,"Language":"zh-Hans","AutomaticUpdates":false}`),
	}
	for name, b := range protected {
		if err := os.WriteFile(name, b, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	protected[input] = sample
	oldFiles := hashes(t, oldRuntime)
	if output, err := runInstaller(t, candidate, ""); err != nil {
		t.Fatalf("upgrade to %s: %v\n%s", kit.Candidate.Version, err, output)
	}
	newRuntime := checkInstalledRuntime(t, install, shortcut, kit.Candidate.Version)
	checkDesktopShortcut(t, desktopLinks)
	assertAbsent(t, defaultInstall)
	if newRuntime == oldRuntime {
		t.Fatal("upgrade reused the previous runtime directory")
	}
	compareHashes(t, hashes(t, oldRuntime), oldFiles)
	verifyInstalledWindow(t, install, work, filepath.Base(input), filepath.Join(data, "apple.heic"), filepath.Join(data, "different-pages.tiff"))
	t.Run("installed-batch", func(t *testing.T) { runInstalledBatch(t, root, newRuntime) })

	// NSIS normally starts a temporary copy and returns immediately. Use an
	// independent copy of the generated uninstaller so we can wait on and
	// check the result of the real uninstall, including its cleanup process.
	uninstaller := copyInstalledUninstaller(t, install)
	before := hashes(t, install)
	func() {
		f, err := os.Open(filepath.Join(install, winupdate.RuntimeLockFile))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		var offset windows.Overlapped
		if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &offset); err != nil {
			t.Fatal(err)
		}
		out, err := runInstaller(t, uninstaller, "_?="+install)
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			t.Fatalf("uninstall should refuse the active install: %v\n%s", err, out)
		}
		compareHashes(t, hashes(t, install), before)
		checkInstalledRegistry(t, kit.Candidate.Version, install)
		if err := windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &offset); err != nil {
			t.Fatal(err)
		}
	}()
	if output, err := runInstaller(t, uninstaller, "_?="+install); err != nil {
		t.Fatalf("uninstall %s: %v\n%s", kit.Candidate.Version, err, output)
	}
	for _, path := range []string{shortcut, filepath.Join(install, "MochiPic.exe"), filepath.Join(install, "Uninstall.exe"),
		filepath.Join(install, "current-runtime"), filepath.Join(install, winupdate.RuntimeLockFile),
		filepath.Join(newRuntime, "MochiPic.exe"), filepath.Join(newRuntime, "libvips-42.dll")} {
		assertAbsent(t, path)
	}
	for _, path := range desktopLinks {
		assertAbsent(t, path)
	}
	checkInstalledRegistry(t, "", install)
	retainedOldFiles := oldFiles
	if _, owned := oldFiles["runtime-files.json"]; owned {
		original := filepath.Join("etc", "fonts", filepath.Base(fontSource))
		retainedOldFiles = map[string][32]byte{original: oldFiles[original]}
	}
	compareHashes(t, hashes(t, oldRuntime), retainedOldFiles)

	// Also exercise the candidate's first install and complete removal,
	// using an empty installation directory with spaces and Unicode.
	fresh := filepath.Join(work, "干净安装 "+kit.Candidate.Version)
	if output, err := runInstaller(t, candidate, "/D="+fresh); err != nil {
		t.Fatalf("clean candidate install: %v\n%s", err, output)
	}
	checkInstalledRuntime(t, fresh, shortcut, kit.Candidate.Version)
	checkDesktopShortcut(t, desktopLinks)
	verifyInstalledWindow(t, fresh, work, filepath.Base(input))
	if output, err := runInstaller(t, copyInstalledUninstaller(t, fresh), "_?="+fresh); err != nil {
		t.Fatalf("clean candidate uninstall: %v\n%s", err, output)
	}
	assertAbsent(t, fresh)
	assertAbsent(t, shortcut)
	for _, path := range desktopLinks {
		assertAbsent(t, path)
	}
	checkInstalledRegistry(t, "", fresh)
	for name, expected := range protected {
		if testkit.Hash(t, name) != sha256.Sum256(expected) {
			t.Errorf("installer lifecycle changed user data: %q", name)
		}
	}
	t.Log("custom install, retained path on upgrade, desktop shortcut, native startup and uninstall verified")
}

func checkDesktopShortcut(t *testing.T, candidates []string) {
	t.Helper()
	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Size() > 0 {
			return
		}
	}
	t.Fatal("missing desktop shortcut")
}

func checkedSetup(t *testing.T, root string, asset installerAsset) string {
	t.Helper()
	if asset.Version == "" || asset.Name != "MochiPic-"+asset.Version+"-windows-x64-setup.exe" || !filepath.IsLocal(asset.Name) || filepath.Base(asset.Name) != asset.Name {
		t.Fatalf("invalid installer asset: %+v", asset)
	}
	path := filepath.Join(root, "Installers", asset.Name)
	if actual := fmt.Sprintf("%x", testkit.Hash(t, path)); actual != asset.SHA256 {
		t.Fatalf("installer %s fails its SHA-256 check", asset.Name)
	}
	return path
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected no file or directory at %q, got %v; this suite needs a fresh Windows user", path, err)
	}
}

func runInstaller(t *testing.T, path, directoryArgument string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "/S")
	if directoryArgument != "" {
		cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: windows.EscapeArg(path) + " /S " + directoryArgument}
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func copyInstalledUninstaller(t *testing.T, install string) string {
	t.Helper()
	copy := filepath.Join(t.TempDir(), "InstalledUninstaller.exe")
	if err := os.WriteFile(copy, readWindowsFixture(t, filepath.Join(install, "Uninstall.exe")), 0o700); err != nil {
		t.Fatal(err)
	}
	return copy
}

func checkInstalledRegistry(t *testing.T, version, install string) {
	t.Helper()
	found := false
	for _, view := range registryViews {
		key, err := registry.OpenKey(registry.CURRENT_USER, installRegistry, registry.QUERY_VALUE|view)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		name, _, verErr := key.GetStringValue("DisplayVersion")
		command, _, cmdErr := key.GetStringValue("UninstallString")
		_ = key.Close()
		if version == "" || verErr != nil || cmdErr != nil || name != version || !strings.EqualFold(command, `"`+filepath.Join(install, "Uninstall.exe")+`"`) {
			t.Fatalf("unexpected installer registry data (%q, %q): %v %v", name, command, verErr, cmdErr)
		}
		found = true
	}
	if version != "" && !found {
		t.Fatalf("install did not register version %s", version)
	}
}

func checkInstalledRuntime(t *testing.T, install, shortcut, version string) string {
	t.Helper()
	checkInstalledRegistry(t, version, install)
	if info, err := os.Stat(shortcut); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		t.Fatalf("missing Start menu shortcut: %v", err)
	}
	store, err := os.OpenRoot(install)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	exe, err := winupdate.Current(store)
	if err != nil {
		t.Fatal(err)
	}
	runtime := filepath.Dir(exe)
	if have, err := winupdate.ReleaseVersion(os.DirFS(runtime)); err != nil || have != version {
		t.Fatalf("selected runtime version %q, expected %q: %v", have, version, err)
	}
	return runtime
}

func verifyInstalledWindow(t *testing.T, install, dir string, files ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(install, "MochiPic.exe"), append([]string{"--verify-window"}, files...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "VIPSHOME=", "FONTCONFIG_FILE=", "MAGICK_CONFIGURE_PATH=", "IMAGEFLOW_CONFIG_DIR=")
	output, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(output), "MyGo native frame rendered with Lobe MyGo") {
		t.Fatalf("installed native window: %v\n%s", err, output)
	}
	t.Log(string(output))
}

func runInstalledBatch(t *testing.T, root, runtime string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, "batch.test.exe"), "-test.v", "-test.count=1", "-test.timeout=4m30s")
	cmd.Dir = filepath.Join(root, "internal", "batch")
	cmd.Env = append(os.Environ(),
		"PATH="+runtime+";"+os.Getenv("PATH"), "IMAGEFLOW_STRESS=1", "VIPSHOME="+runtime,
		"FONTCONFIG_FILE="+filepath.Join(runtime, "etc", "fonts", "fonts.conf"),
		"MAGICK_CONFIGURE_PATH="+filepath.Join(runtime, "etc", "ImageMagick-7"))
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("batch verification using installed runtime: %v\n%s", err, output)
	} else {
		t.Log(string(output))
	}
}
