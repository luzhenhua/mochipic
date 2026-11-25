package files

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"imageflow/internal/l10n"

	"golang.org/x/sys/windows"
)

const directoryAccess = windows.FILE_GENERIC_READ | windows.FILE_TRAVERSE
const directoryShare = windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE

func openAbsolute(path string) (*os.File, error) {
	volume := filepath.VolumeName(path)
	if volume == "" || !filepath.IsAbs(path) || strings.HasPrefix(volume, `\\.\`) {
		return nil, l10n.New("error.absoluteOutputPath")
	}
	rootPath := volume + `\`
	p, err := windows.UTF16PtrFromString(rootPath)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, directoryAccess, directoryShare, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	root, err := checkedWindowsFile(h, rootPath)
	if err != nil {
		return nil, err
	}
	if filepath.Clean(path) == filepath.Clean(rootPath) {
		return root, nil
	}
	defer root.Close()
	return openTree(root, strings.TrimPrefix(path, rootPath), false)
}

func checkedWindowsFile(h windows.Handle, name string) (*os.File, error) {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		windows.CloseHandle(h)
		return nil, err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		windows.CloseHandle(h)
		return nil, l10n.New("error.unsafeSubfolder")
	}
	return os.NewFile(uintptr(h), name), nil
}

// Resolve each component relative to the open parent and never follow a
// junction or symbolic link inserted into the output tree.
func openRelative(root *os.File, name string, access, share, disposition, options uint32) (*os.File, error) {
	ntName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	attrs := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(root.Fd()), ObjectName: ntName,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&handle, access, &attrs, &status, nil, 0, share, disposition,
		options|windows.FILE_SYNCHRONOUS_IO_NONALERT|windows.FILE_OPEN_REPARSE_POINT, 0, 0)
	if err != nil {
		return nil, winFileError(err)
	}
	return checkedWindowsFile(handle, filepath.Join(root.Name(), name))
}

func winFileError(err error) error {
	var status windows.NTStatus
	if errors.As(err, &status) {
		return status.Errno()
	}
	return err
}

func reopenDirectory(root *os.File, access uint32) (*os.File, error) {
	// Reopen the pinned directory itself with a fresh enumeration cursor.
	// A zero-length relative name keeps resolution on this handle.
	return openRelative(root, "", access, directoryShare, windows.FILE_OPEN, windows.FILE_DIRECTORY_FILE)
}

func openTree(root *os.File, rel string, create bool) (*os.File, error) {
	if err := validateRelativeDir(rel); err != nil {
		return nil, err
	}
	if rel == "" || rel == "." {
		return reopenDirectory(root, directoryAccess)
	}
	dir := root
	disposition := uint32(windows.FILE_OPEN)
	if create {
		disposition = windows.FILE_OPEN_IF
	}
	for _, part := range strings.Split(rel, `\`) {
		next, err := openRelative(dir, part, directoryAccess, directoryShare, disposition, windows.FILE_DIRECTORY_FILE)
		if dir != root {
			_ = dir.Close()
		}
		if err != nil {
			return nil, err
		}
		dir = next
	}
	return dir, nil
}

func readFolderNames(dir *os.File) ([]string, error) {
	f, err := reopenDirectory(dir, directoryAccess)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return f.Readdirnames(-1)
}

func checkFolderAccess(dir *os.File) error {
	f, err := reopenDirectory(dir, directoryAccess|windows.FILE_WRITE_DATA)
	if err != nil {
		return err
	}
	return f.Close()
}

func createTemp(dir *os.File, name string) (*os.File, error) {
	return openRelative(dir, name, windows.FILE_GENERIC_WRITE|windows.FILE_READ_ATTRIBUTES|windows.DELETE,
		0, windows.FILE_CREATE, windows.FILE_NON_DIRECTORY_FILE)
}

func closeTemp(_ *os.File, _ string, file *os.File, published bool) error {
	var err error
	if !published {
		deleteFile := uint32(1)
		err = windows.SetFileInformationByHandle(windows.Handle(file.Fd()), windows.FileDispositionInfo,
			(*byte)(unsafe.Pointer(&deleteFile)), uint32(unsafe.Sizeof(deleteFile)))
	}
	return errors.Join(err, file.Close())
}

func publishTemp(dir, file *os.File, _ string, target string) error {
	name, err := windows.UTF16FromString(target)
	if err != nil {
		return err
	}
	type renameInfo struct {
		replace   uint32
		root      windows.Handle
		nameBytes uint32
		name      uint16
	}
	var layout renameInfo
	start := int(unsafe.Offsetof(layout.name))
	length := (len(name) - 1) * 2
	// Resolve the final name against the same verified directory handle used
	// to create the temporary file, and leave ReplaceIfExists false.
	buf := make([]byte, int(unsafe.Sizeof(layout))+length)
	header := (*renameInfo)(unsafe.Pointer(&buf[0]))
	header.root = windows.Handle(dir.Fd())
	header.nameBytes = uint32(length)
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(&buf[start])), len(name)-1), name)
	var status windows.IO_STATUS_BLOCK
	return winFileError(windows.NtSetInformationFile(windows.Handle(file.Fd()), &status, &buf[0], uint32(len(buf)), windows.FileRenameInformation))
}
