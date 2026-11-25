package winupdate

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"imageflow/internal/testkit"
)

func testStore(t *testing.T) *os.Root {
	t.Helper()
	store, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestSwitchRuntimeWithoutMovingPriorFiles(t *testing.T) {
	store := testStore(t)
	old, err := Install(t.Context(), store, fstest.MapFS{
		appExecutable:          {Data: []byte("old executable")},
		"etc/fonts/fonts.conf": {Data: []byte("old configuration")},
		"lib/old-codec.dll":    {Data: []byte("private obsolete DLL")},
	})
	if err != nil {
		t.Fatal(err)
	}
	original := filepath.Join(filepath.Dir(old), "etc", "fonts", "原图.jpg")
	if err := os.WriteFile(original, []byte("untouched image bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	next, err := Install(t.Context(), store, fstest.MapFS{
		appExecutable:          {Data: []byte("new executable")},
		"etc/fonts/fonts.conf": {Data: []byte("new configuration")},
		"lib/new-codec.dll":    {Data: []byte("new private DLL")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if current, err := Current(store); err != nil || current != next || current == old {
		t.Fatalf("current executable %q: %v", current, err)
	}
	for path, want := range map[string]string{
		old:      "old executable",
		next:     "new executable",
		original: "untouched image bytes",
		filepath.Join(filepath.Dir(old), "etc", "fonts", "fonts.conf"):  "old configuration",
		filepath.Join(filepath.Dir(next), "etc", "fonts", "fonts.conf"): "new configuration",
	} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Errorf("%q: got %q, want %q, error %v", path, data, want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(next), "lib", "old-codec.dll")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("next runtime exposed an obsolete library: %v", err)
	}
}

func TestInvalidRuntimeCannotReplaceCurrent(t *testing.T) {
	store := testStore(t)
	old, err := Install(t.Context(), store, fstest.MapFS{appExecutable: {Data: []byte("working app")}})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		bundle fstest.MapFS
	}{
		{"missing executable", fstest.MapFS{"a.txt": {Data: []byte("file")}}},
		{"empty executable", fstest.MapFS{appExecutable: {}}},
		{"archive link", fstest.MapFS{appExecutable: {Mode: fs.ModeSymlink, Data: []byte("outside")}}},
		{"Windows alternate stream", fstest.MapFS{appExecutable: {Data: []byte("valid")}, "file.txt:stream": {}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Install(t.Context(), store, tc.bundle); err == nil {
				t.Fatal("accepted an invalid runtime")
			}
			if current, err := Current(store); err != nil || current != old {
				t.Fatalf("active runtime changed to %q: %v", current, err)
			}
		})
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := Install(canceled, store, fstest.MapFS{appExecutable: {Data: []byte("valid")}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled install returned %v", err)
	}
	if entries, err := fs.ReadDir(store.FS(), "."); err != nil || len(entries) != 2 {
		t.Fatalf("staging files left in store: %v, error %v", entries, err)
	}
}

func TestRejectUnownedActiveRecord(t *testing.T) {
	store := testStore(t)
	const data = "../unrelated-file\n"
	if err := store.WriteFile(activeFile, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Current(store); err == nil {
		t.Fatal("used an invalid active record")
	}
	if _, err := Install(t.Context(), store, fstest.MapFS{appExecutable: {Data: []byte("valid")}}); err == nil {
		t.Fatal("overwrote a record not owned by the updater")
	}
	if after, err := store.ReadFile(activeFile); err != nil || string(after) != data {
		t.Fatalf("unowned record changed: %q %v", after, err)
	}
}

type cancelOnRead struct {
	fs.FS
	cancel context.CancelFunc
}

func (c cancelOnRead) Open(name string) (fs.File, error) {
	if name == appExecutable {
		c.cancel()
	}
	return c.FS.Open(name)
}

func TestInterruptedCopyKeepsActiveRuntime(t *testing.T) {
	store := testStore(t)
	old, err := Install(t.Context(), store, fstest.MapFS{appExecutable: {Data: []byte("working app")}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	changed := cancelOnRead{
		FS:     fstest.MapFS{appExecutable: {Data: []byte("new app")}, "lib/codec.dll": {Data: []byte("new codec")}},
		cancel: cancel,
	}
	if _, err := Install(ctx, store, changed); !errors.Is(err, context.Canceled) {
		t.Fatalf("interrupted staging returned %v", err)
	}
	if current, err := Current(store); err != nil || current != old {
		t.Fatalf("active runtime changed to %q: %v", current, err)
	}
	if entries, err := fs.ReadDir(store.FS(), "."); err != nil || len(entries) != 2 {
		t.Fatalf("interrupted stage left entries: %v, error %v", entries, err)
	}
}

func TestRejectRuntimeDirectoryLink(t *testing.T) {
	store := testStore(t)
	staged := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(staged, appExecutable), []byte("working app"), 0o700); err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(outside, "原图.png")
	if err := os.WriteFile(asset, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := testkit.Hash(t, asset)
	testkit.DirectoryLink(t, outside, filepath.Join(staged, "etc"))
	if _, err := Install(t.Context(), store, os.DirFS(staged)); err == nil {
		t.Fatal("accepted a runtime directory link")
	}
	if entries, err := fs.ReadDir(store.FS(), "."); err != nil || len(entries) != 0 {
		t.Fatalf("invalid stage left entries: %v, error %v", entries, err)
	}
	if testkit.Hash(t, asset) != before {
		t.Fatal("file outside the package changed")
	}
}

func TestReleaseVersionRequiresCanonicalStamp(t *testing.T) {
	for _, invalid := range []string{"", "0.0.01", "v0.0.19", "0.0.19\n0.0.20", "99999999999999999999999999999999999.0.1"} {
		if _, err := ReleaseVersion(fstest.MapFS{VersionFile: {Data: []byte(invalid)}}); err == nil {
			t.Errorf("accepted runtime version %q", invalid)
		}
	}
	if version, err := ReleaseVersion(fstest.MapFS{VersionFile: {Data: []byte("0.0.20\n")}}); err != nil || version != "0.0.20" {
		t.Fatalf("signed runtime version = %q, %v", version, err)
	}
}
