package imaging

import (
	"golang.org/x/sys/unix"
	"os"
)

func openInput(path string, maxBytes int) (*os.File, int64, error) {
	// Open only regular files and never follow a substituted symlink.
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NONBLOCK|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, 0, err
	}
	f := os.NewFile(uintptr(fd), path)
	return checkInputFile(f, maxBytes)
}
