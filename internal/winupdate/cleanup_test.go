package winupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"imageflow/internal/testkit"
)

func installRecorded(t *testing.T, store *os.Root, program string, files map[string][]byte) string {
	t.Helper()
	image := fstest.MapFS{}
	hashes := make(map[string]string)
	files[appExecutable] = []byte(program)
	files[VersionFile] = []byte("0.0.21\n")
	for name, data := range files {
		image[name] = &fstest.MapFile{Data: data}
		hash := sha256.Sum256(data)
		hashes[name] = hex.EncodeToString(hash[:])
	}
	data, err := json.Marshal(map[string]any{"schema": 1, "files": hashes})
	if err != nil {
		t.Fatal(err)
	}
	image[receiptFile] = &fstest.MapFile{Data: data}
	exe, err := Install(t.Context(), store, image)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(exe)
}

func TestUninstallRemovesOwnedFilesAcrossRuntimes(t *testing.T) {
	store := testStore(t)
	old := installRecorded(t, store, "old executable", map[string][]byte{
		"libold.dll":                         []byte("old DLL"),
		"etc/fonts/fonts.conf":               []byte("original packaged configuration"),
		"lib/vips-modules-8.18/vips-jxl.dll": []byte("old JPEG XL plugin"),
	})
	next := installRecorded(t, store, "new executable", map[string][]byte{
		"libnew.dll":                     []byte("new DLL"),
		"vips-modules-8.18/vips-jxl.dll": []byte("new JPEG XL plugin"),
	})
	legacy := filepath.Join(store.Name(), "runtime-CCCCCCCCCCCCCCCCCCCCCCCCCC")
	if err := os.Mkdir(legacy, 0o700); err != nil {
		t.Fatal(err)
	}
	data := map[string]string{
		filepath.Join(old, "etc", "fonts", "客户原图.png"):   "user original",
		filepath.Join(old, "etc", "fonts", "fonts.conf"): "edited font configuration",
		filepath.Join(store.Name(), "导出.webp"):           "user export",
		filepath.Join(legacy, appExecutable):             "unrecorded older installation",
	}
	for name, content := range data {
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		if err := UninstallRuntimes(store); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{next, filepath.Join(old, appExecutable), filepath.Join(old, "libold.dll"),
		filepath.Join(old, "lib", "vips-modules-8.18", "vips-jxl.dll"),
		filepath.Join(old, receiptFile), filepath.Join(store.Name(), activeFile)} {
		if _, err := os.Lstat(name); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("owned install file still present: %q, error %v", name, err)
		}
	}
	for name, want := range data {
		if actual, err := os.ReadFile(name); err != nil || string(actual) != want {
			t.Errorf("unowned file changed: %q, error %v", name, err)
		}
	}
}

func TestUninstallRejectsReceiptThatClaimsImageData(t *testing.T) {
	store := testStore(t)
	image := []byte("user source image")
	runtime := installRecorded(t, store, "not owned without a valid receipt", map[string][]byte{"etc/fonts/original.jxl": image})
	if err := UninstallRuntimes(store); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(runtime, "etc", "fonts", "original.jxl")); err != nil || string(got) != string(image) {
		t.Fatalf("malformed receipt affected the input image: %q %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(runtime, appExecutable)); err != nil {
		t.Fatalf("uninstall acted on a malformed receipt: %v", err)
	}
}

func TestUninstallNeverFollowsDirectoryLink(t *testing.T) {
	store := testStore(t)
	data := []byte("same as packaged configuration")
	runtime := installRecorded(t, store, "test executable", map[string][]byte{"etc/fonts/fonts.conf": data})
	fonts := filepath.Join(runtime, "etc", "fonts")
	if err := os.Remove(filepath.Join(fonts, "fonts.conf")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(fonts); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	outside := filepath.Join(target, "fonts.conf")
	if err := os.WriteFile(outside, data, 0o600); err != nil {
		t.Fatal(err)
	}
	testkit.DirectoryLink(t, target, fonts)
	if err := UninstallRuntimes(store); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(outside); err != nil || string(got) != string(data) {
		t.Fatalf("uninstall changed data through a directory link: %q %v", got, err)
	}
	if _, err := os.Lstat(fonts); err != nil {
		t.Fatalf("uninstall changed the directory link: %v", err)
	}
}
