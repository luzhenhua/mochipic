//go:build windows && amd64

package releaseqa

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"imageflow/internal/testkit"
	"imageflow/internal/winupdate"

	"golang.org/x/sys/windows"
)

const windowsManifest = "update-windows-amd64.json"

// The test launcher is built from the production source with a temporary
// public key and a fixed loopback feed. The regular product never uses it.
var fixtureAddress string

type windowsUpdateManifest struct {
	Version   string `json:"version"`
	URL       string `json:"url"`
	Signature string `json:"signature"`
	Size      int64  `json:"size"`
}

func TestWindowsSignedUpdate(t *testing.T) {
	root := os.Getenv("MOCHIPIC_WINDOWS_UPDATE_QA")
	if root == "" {
		t.Skip("use the offline Windows update QA bundle")
	}
	fixtures := filepath.Join(root, "Updates")
	var manifest windowsUpdateManifest
	if err := json.Unmarshal(readWindowsFixture(t, filepath.Join(fixtures, windowsManifest)), &manifest); err != nil {
		t.Fatal(err)
	}
	baseline := filepath.Join(root, "Runtime")
	version, err := winupdate.ReleaseVersion(os.DirFS(baseline))
	if err != nil || manifest.Version != version ||
		manifest.URL != "mochipic-"+version+"-windows-amd64.tar.gz" {
		t.Fatalf("invalid runtime fixture manifest: %+v, %v", manifest, err)
	}
	archive := readWindowsFixture(t, filepath.Join(fixtures, manifest.URL))
	if int64(len(archive)) != manifest.Size {
		t.Fatal("update archive size differs from its manifest")
	}
	broken := append([]byte(nil), archive...)
	broken[len(broken)/2] ^= 1

	var corrupt atomic.Bool
	var spoofVersion atomic.Bool
	listener, err := net.Listen("tcp4", fixtureAddress)
	if err != nil {
		t.Fatalf("start loopback update fixture: %v", err)
	}
	mux := http.NewServeMux()
	server := httptest.NewUnstartedServer(mux)
	_ = server.Listener.Close()
	server.Listener = listener
	mux.HandleFunc("/manifest", func(w http.ResponseWriter, _ *http.Request) {
		m := manifest
		m.URL = server.URL + "/archive"
		if spoofVersion.Load() {
			m.Version = "999999.0.0"
		}
		_ = json.NewEncoder(w).Encode(m)
	})
	mux.HandleFunc("/archive", func(w http.ResponseWriter, _ *http.Request) {
		if corrupt.Load() {
			_, _ = w.Write(broken)
			return
		}
		_, _ = w.Write(archive)
	})
	server.Start()
	defer server.Close()

	install := filepath.Join(t.TempDir(), "安装 测试")
	if err := os.Mkdir(install, 0o700); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(install, "MochiPic.exe")
	if err := os.WriteFile(launcher, readWindowsFixture(t, filepath.Join(root, "Launcher.exe")), 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := os.OpenRoot(install)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	previous, err := winupdate.Install(t.Context(), store, os.DirFS(baseline))
	if err != nil {
		t.Fatal(err)
	}
	oldRuntime := filepath.Dir(previous)
	sample := filepath.Join(root, "Samples", "still.jxl")
	image := readWindowsFixture(t, sample)

	// Only the new executable can start. This also proves that the stable
	// launcher uses the newly selected runtime after applying the update.
	artifacts := map[string][]byte{
		previous: []byte("previous runtime sentinel"),
		filepath.Join(oldRuntime, winupdate.VersionFile):        []byte("0.0.0\n"),
		filepath.Join(install, winupdate.RuntimeLockFile):       {},
		filepath.Join(install, winupdate.UpdateLockFile):        {},
		filepath.Join(install, "Uninstall.exe"):                 []byte("uninstaller sentinel"),
		filepath.Join(install, "用户导出.jxl"):                      image,
		filepath.Join(oldRuntime, "etc", "fonts", "客户原图.jxl"):   image,
		filepath.Join(oldRuntime, "etc", "fonts", "fonts.conf"): []byte("user-edited configuration"),
	}
	for path, data := range artifacts {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	before := hashes(t, install)
	oldFiles := hashes(t, oldRuntime)
	run := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, launcher, args...)
		cmd.Env = append(os.Environ(), "VIPSHOME=", "FONTCONFIG_FILE=", "MAGICK_CONFIGURE_PATH=", "IMAGEFLOW_CONFIG_DIR="+t.TempDir())
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	expectFailure := func(want string, args ...string) {
		t.Helper()
		output, err := run(args...)
		if err == nil || !strings.Contains(output, want) {
			t.Fatalf("%s: %v\n%s", want, err, output)
		}
		compareHashes(t, hashes(t, install), before)
		t.Logf("rejected: %s", want)
	}
	expectFailure("isolated staging directory", winupdate.VerifyCommand, "0.0.0", version)
	corrupt.Store(true)
	expectFailure("not signed with the app's key", winupdate.ApplyCommand, version)
	corrupt.Store(false)
	spoofVersion.Store(true)
	expectFailure("authenticated runtime does not match update version", winupdate.ApplyCommand, "999999.0.0")
	spoofVersion.Store(false)
	if output, err := run(winupdate.ApplyCommand, version); err != nil {
		t.Fatalf("apply the signed Windows update: %v\n%s", err, output)
	}
	selected, err := winupdate.Current(store)
	if err != nil || selected == previous {
		t.Fatalf("new runtime not selected: %q %v", selected, err)
	}
	compareHashes(t, hashes(t, oldRuntime), oldFiles)
	compareHashes(t, hashes(t, filepath.Dir(selected)), hashes(t, baseline))
	for path, data := range artifacts {
		if testkit.Hash(t, path) != sha256.Sum256(data) {
			t.Fatalf("previous installation file or photo modified: %q", path)
		}
	}
	before = hashes(t, install)
	expectFailure("requested update is no longer available", winupdate.ApplyCommand, version)
	if output, err := run("--verify-window", sample); err != nil ||
		!strings.Contains(output, "MyGo native frame rendered with Lobe MyGo") {
		t.Fatalf("updated app did not render a native frame: %v\n%s", err, output)
	}
	t.Logf("updated to %s and launched the native application with prior files intact", version)

	before = hashes(t, install)
	func() {
		lock, err := os.Open(filepath.Join(install, winupdate.RuntimeLockFile))
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Close()
		var offset windows.Overlapped
		if err := windows.LockFileEx(windows.Handle(lock.Fd()), windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &offset); err != nil {
			t.Fatal(err)
		}
		expectFailure("MochiPic is using the installation directory", winupdate.CleanupCommand)
		if err := windows.UnlockFileEx(windows.Handle(lock.Fd()), 0, 1, 0, &offset); err != nil {
			t.Fatal(err)
		}
	}()
	if output, err := run(winupdate.CleanupCommand); err != nil {
		t.Fatalf("uninstall signed runtimes: %v\n%s", err, output)
	}
	for name := range hashes(t, baseline) {
		if _, err := os.Lstat(filepath.Join(filepath.Dir(selected), name)); !os.IsNotExist(err) {
			t.Errorf("new runtime file not cleaned: %q, %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(oldRuntime, "libvips-42.dll")); !os.IsNotExist(err) {
		t.Errorf("old runtime DLL not cleaned: %v", err)
	}
	for name, data := range artifacts {
		if filepath.Base(name) == winupdate.RuntimeLockFile || filepath.Base(name) == winupdate.UpdateLockFile {
			if _, err := os.Stat(name); !os.IsNotExist(err) {
				t.Errorf("coordination file not cleaned: %q, %v", name, err)
			}
			continue
		}
		if testkit.Hash(t, name) != sha256.Sum256(data) {
			t.Fatalf("uninstall touched user or modified data: %q", name)
		}
	}
	t.Log("uninstall removed owned runtimes and preserved photos and modified files")
}

func readWindowsFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func hashes(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	sums := make(map[string][32]byte)
	err := fs.WalkDir(os.DirFS(root), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Type().IsRegular() {
			sums[filepath.FromSlash(rel)] = testkit.Hash(t, filepath.Join(root, rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return sums
}

func compareHashes(t *testing.T, actual, expected map[string][32]byte) {
	t.Helper()
	if len(actual) != len(expected) {
		t.Errorf("tree contains %d files, expected %d", len(actual), len(expected))
	}
	for name, sum := range expected {
		if got, exists := actual[name]; !exists || got != sum {
			t.Errorf("missing or altered file %q", name)
		}
	}
}
