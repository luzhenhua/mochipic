//go:build !windows

package winupdate

import (
	"crypto/sha256"
	"os"
)

// Used by the portable file-ownership tests. The Windows implementation
// commits deletion through the verified native file handle.
func removeIdentical(dir *os.Root, name string, info os.FileInfo, want [sha256.Size]byte) error {
	file, err := dir.Open(name)
	if err != nil {
		return err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return err
	}
	match, err := matchingContents(file, want)
	if err != nil || !match {
		return err
	}
	return dir.Remove(name)
}
