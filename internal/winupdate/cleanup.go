package winupdate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"strings"
)

const receiptFile = "runtime-files.json"
const maxReceipt = 256 << 10

// UninstallRuntimes removes byte-identical packaged files from each recorded
// runtime. The caller must exclude concurrent launches and activations. A
// runtime without a valid receipt is treated as unowned.
func UninstallRuntimes(store *os.Root) error {
	entries, err := fs.ReadDir(store.FS(), ".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() && validRuntimeID(entry.Name()) {
			if err := uninstallRuntime(store, entry.Name()); err != nil {
				return fmt.Errorf("remove runtime %q: %w", entry.Name(), err)
			}
		}
	}
	if id, err := currentID(store); err == nil {
		return removeOwnedFile(store, activeFile, sha256.Sum256([]byte(id+"\n")))
	}
	return nil
}

// RemoveEmptyLocks clears the two installer-owned coordination files after
// their handles have closed. A file containing any data is left in place.
func RemoveEmptyLocks(store *os.Root) error {
	empty := sha256.Sum256(nil)
	return errors.Join(removeOwnedFile(store, RuntimeLockFile, empty), removeOwnedFile(store, UpdateLockFile, empty))
}

type receipt struct {
	files map[string][sha256.Size]byte
	hash  [sha256.Size]byte
}

func readReceipt(runtime *os.Root) (receipt, bool) {
	var r receipt
	info, err := runtime.Lstat(receiptFile)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxReceipt {
		return r, false
	}
	f, err := runtime.Open(receiptFile)
	if err != nil {
		return r, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxReceipt+1))
	if err != nil || len(data) > maxReceipt {
		return r, false
	}
	var doc struct {
		Schema int
		Files  map[string]string
	}
	if err := json.Unmarshal(data, &doc); err != nil || doc.Schema != 1 ||
		doc.Files[appExecutable] == "" || doc.Files[VersionFile] == "" {
		return r, false
	}
	r.files = make(map[string][sha256.Size]byte, len(doc.Files))
	seen := make(map[string]bool, len(doc.Files))
	for name, digest := range doc.Files {
		lower := strings.ToLower(name)
		if !runtimeFileName(name) || seen[lower] {
			return receipt{}, false
		}
		var sum [sha256.Size]byte
		if n, err := hex.Decode(sum[:], []byte(digest)); err != nil || n != len(sum) || len(digest) != 2*len(sum) {
			return receipt{}, false
		}
		seen[lower] = true
		r.files[name] = sum
	}
	r.hash = sha256.Sum256(data)
	return r, true
}

// The receipt must never be able to describe user image formats, even if
// someone manually edits it. The runtime layout below matches the packager.
func runtimeFileName(name string) bool {
	if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:<>\"|?*") {
		return false
	}
	parent, base := path.Split(name)
	for _, c := range []byte(base) {
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.ContainsRune(".+-_", rune(c))) {
			return false
		}
	}
	switch parent {
	case "":
		return base == appExecutable || base == VersionFile || base == "Windows-Test.md" || strings.HasSuffix(base, ".dll")
	case "etc/fonts/":
		return base == "fonts.conf"
	case "etc/fonts/conf.d/":
		return base == "README" || strings.HasSuffix(base, ".conf")
	case "etc/ImageMagick-7/":
		return base == "policy.xml"
	case "ThirdParty/":
		return base == "LICENSE" || base == "README.md" || base == "PhotoCredits.txt" || base == "codec-build.txt" ||
			base == "libde265-COPYING" || base == "libheif-COPYING" || base == "runtime-dlls.txt" || base == "versions.json"
	}
	moduleDir := strings.TrimPrefix(parent, "lib/") // Older installers kept plugins under lib/.
	return strings.HasPrefix(moduleDir, "vips-modules-") && strings.Count(moduleDir, "/") == 1 &&
		(base == "vips-jxl.dll" || base == "vips-magick.dll")
}

func uninstallRuntime(parent *os.Root, name string) error {
	runtime, err := openRecordedDirectory(parent, name)
	if err != nil || runtime == nil {
		return err
	}
	r, known := readReceipt(runtime)
	if !known {
		return runtime.Close()
	}
	err = removeRecordedTree(runtime, r.files)
	if err == nil {
		err = removeOwnedFile(runtime, receiptFile, r.hash)
	}
	err = errors.Join(err, runtime.Close())
	if err != nil {
		return err
	}
	// Remove only empty directories. Photos or modified resources left in a
	// directory keep it intact.
	_ = parent.Remove(name)
	return nil
}

func removeRecordedTree(dir *os.Root, files map[string][sha256.Size]byte) error {
	subdirs := make(map[string]map[string][sha256.Size]byte)
	for name, sum := range files {
		child, rest, nested := strings.Cut(name, "/")
		if !nested {
			if err := removeOwnedFile(dir, name, sum); err != nil {
				return fmt.Errorf("remove %q: %w", name, err)
			}
			continue
		}
		if subdirs[child] == nil {
			subdirs[child] = make(map[string][sha256.Size]byte)
		}
		subdirs[child][rest] = sum
	}
	for name, children := range subdirs {
		nested, err := openRecordedDirectory(dir, name)
		if err != nil {
			return err
		}
		if nested == nil {
			continue
		}
		if err := errors.Join(removeRecordedTree(nested, children), nested.Close()); err != nil {
			return err
		}
		_ = dir.Remove(name)
	}
	return nil
}

func openRecordedDirectory(parent *os.Root, name string) (*os.Root, error) {
	info, err := parent.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.IsDir() {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	dir, err := parent.OpenRoot(name)
	if err != nil {
		return nil, err
	}
	opened, err := dir.Stat(".")
	if err != nil || !os.SameFile(info, opened) {
		_ = dir.Close()
		return nil, err
	}
	return dir, nil
}

func matchingContents(file *os.File, want [sha256.Size]byte) (bool, error) {
	got := sha256.New()
	if _, err := io.Copy(got, file); err != nil {
		return false, err
	}
	return [sha256.Size]byte(got.Sum(nil)) == want, nil
}

func removeOwnedFile(dir *os.Root, name string, want [sha256.Size]byte) error {
	info, err := dir.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) || err == nil && !info.Mode().IsRegular() {
		return nil
	}
	if err != nil {
		return err
	}
	return removeIdentical(dir, name, info, want)
}
