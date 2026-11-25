package files

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"imageflow/internal/testkit"
)

func TestWindowsReopenDirectoryHasIndependentEnumeration(t *testing.T) {
	path := t.TempDir()
	if err := os.WriteFile(filepath.Join(path, "泵.png"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := openAbsolute(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	for range 2 {
		names, err := readFolderNames(root)
		if err != nil || !slices.Equal(names, []string{"泵.png"}) {
			t.Fatalf("enumerate pinned directory: %v, %v", names, err)
		}
	}
	if err := checkFolderAccess(root); err != nil {
		t.Fatalf("check writable directory: %v", err)
	}
}

func TestWindowsRenameInPlaceIsExclusive(t *testing.T) {
	path := t.TempDir()
	root, err := openAbsolute(path)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	existing := filepath.Join(path, "泵.png")
	if err := os.WriteFile(existing, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	before := testkit.Hash(t, existing)
	tmp, file, err := tempAt(root)
	if err != nil {
		t.Fatal(err)
	}
	published := false
	defer func() {
		if !published {
			if err := closeTemp(root, tmp, file, false); err != nil {
				t.Error(err)
			}
		}
		assertNoTemp(t, path)
	}()
	if _, err := file.WriteString("export"); err != nil {
		t.Fatal(err)
	}
	if err := publishTemp(root, file, tmp, filepath.Base(existing)); !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected the kernel to reject an existing file: %v", err)
	}
	if testkit.Hash(t, existing) != before {
		t.Fatal("existing file changed")
	}
	name := strings.Repeat("泵", 120) + ".png"
	if err := publishTemp(root, file, tmp, name); err != nil {
		t.Fatal(err)
	}
	published = true
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(path, name)); err != nil || string(got) != "export" {
		t.Fatalf("published content = %q, error = %v", got, err)
	}
}
