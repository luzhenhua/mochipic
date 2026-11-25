package winupdate

import (
	"crypto/sha256"
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

func removeIdentical(dir *os.Root, name string, info os.FileInfo, want [sha256.Size]byte) error {
	parent, err := dir.Open(".")
	if err != nil {
		return err
	}
	defer parent.Close()
	path, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return err
	}
	attrs := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(parent.Fd()), ObjectName: path,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	attrs.Length = uint32(unsafe.Sizeof(attrs))
	var handle windows.Handle
	var status windows.IO_STATUS_BLOCK
	// Hash the opened file and delete using that same handle. Sharing only
	// reads prevents a concurrent writer from changing the verified bytes.
	err = windows.NtCreateFile(&handle, windows.FILE_GENERIC_READ|windows.DELETE, &attrs, &status, nil, 0,
		windows.FILE_SHARE_READ, windows.FILE_OPEN, windows.FILE_NON_DIRECTORY_FILE|windows.FILE_OPEN_REPARSE_POINT|
			windows.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	if err != nil {
		var status windows.NTStatus
		if errors.As(err, &status) {
			err = status.Errno()
		}
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	file := os.NewFile(uintptr(handle), name)
	defer file.Close()
	var native windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &native); err != nil {
		return err
	}
	if native.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil
	}
	opened, err := file.Stat()
	if err != nil || !os.SameFile(info, opened) {
		return err
	}
	match, err := matchingContents(file, want)
	if err != nil || !match {
		return err
	}
	deleteFile := byte(1)
	return windows.SetFileInformationByHandle(handle, windows.FileDispositionInfo, &deleteFile, 1)
}
