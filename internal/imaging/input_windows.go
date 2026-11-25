package imaging

import (
	"os"
	"path/filepath"
	"strings"

	"imageflow/internal/l10n"

	"golang.org/x/sys/windows"
)

func openInput(path string, maxBytes int) (*os.File, int64, error) {
	// Open read-only and inspect the handle before reading through it. In
	// particular, a substituted reparse point never becomes an image input.
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, 0, err
	}
	if strings.HasPrefix(absolute, `\\.\`) {
		return nil, 0, os.ErrInvalid
	}
	switch {
	case strings.HasPrefix(absolute, `\\?\`):
	case strings.HasPrefix(absolute, `\\`):
		absolute = `\\?\UNC\` + absolute[2:]
	default:
		absolute = `\\?\` + absolute
	}
	name, err := windows.UTF16PtrFromString(absolute)
	if err != nil {
		return nil, 0, err
	}
	h, err := windows.CreateFile(name, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_SEQUENTIAL_SCAN, 0)
	if err != nil {
		return nil, 0, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		windows.CloseHandle(h)
		return nil, 0, err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY) != 0 {
		windows.CloseHandle(h)
		return nil, 0, l10n.Error("error.inputFileLimit", l10n.Args{"megabytes": maxBytes >> 20})
	}
	return checkInputFile(os.NewFile(uintptr(h), path), maxBytes)
}
