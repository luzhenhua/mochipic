package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const dirFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

func openAbsolute(path string) (*os.File, error) {
	root, err := os.Open("/")
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return openTree(root, strings.TrimPrefix(path, "/"), false)
}

func openTree(root *os.File, rel string, create bool) (*os.File, error) {
	if err := validateRelativeDir(rel); err != nil {
		return nil, err
	}
	fd, err := unix.Openat(int(root.Fd()), ".", dirFlags, 0)
	if err != nil {
		return nil, err
	}
	if rel != "" && rel != "." {
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			next, e := unix.Openat(fd, part, dirFlags, 0)
			if create && errors.Is(e, unix.ENOENT) {
				if e = unix.Mkdirat(fd, part, 0o755); e == nil || errors.Is(e, unix.EEXIST) {
					next, e = unix.Openat(fd, part, dirFlags, 0)
				}
			}
			_ = unix.Close(fd)
			if e != nil {
				return nil, e
			}
			fd = next
		}
	}
	return os.NewFile(uintptr(fd), filepath.Join(root.Name(), rel)), nil
}

// Open a fresh descriptor so each directory listing starts at the beginning.
func readFolderNames(dir *os.File) ([]string, error) {
	fd, err := unix.Openat(int(dir.Fd()), ".", dirFlags, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), dir.Name())
	defer f.Close()
	return f.Readdirnames(-1)
}

func checkFolderAccess(dir *os.File) error {
	return unix.Faccessat(int(dir.Fd()), ".", unix.W_OK|unix.X_OK, 0)
}

func createTemp(dir *os.File, name string) (*os.File, error) {
	fd, err := unix.Openat(int(dir.Fd()), name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func closeTemp(dir *os.File, name string, file *os.File, _ bool) error {
	err := unix.Unlinkat(int(dir.Fd()), name, 0)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return errors.Join(file.Close(), err)
}

func publishTemp(dir *os.File, _ *os.File, name, target string) error {
	fd := int(dir.Fd())
	err := unix.RenameatxNp(fd, name, fd, target, unix.RENAME_EXCL)
	if errors.Is(err, unix.ENOTSUP) || errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) {
		err = unix.Linkat(fd, name, fd, target, 0)
	}
	return err
}
