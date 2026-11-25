//go:build !windows

package testkit

import (
	"os"
	"testing"
)

func DirectoryLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}
