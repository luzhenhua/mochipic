//go:build !windows

package files

import (
	"os"
	"testing"
)

func denyCreatingFiles(t *testing.T, path string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root can write the protected test directory")
	}
	if err := os.Chmod(path, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(path, 0o700); err != nil {
			t.Error(err)
		}
	})
}
