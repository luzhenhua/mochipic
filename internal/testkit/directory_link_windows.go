package testkit

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// A junction can be created by a standard user on NTFS.
func DirectoryLink(t *testing.T, target, link string) {
	t.Helper()
	target, err := filepath.Abs(target)
	if err != nil {
		t.Fatal(err)
	}
	substitute, err := windows.UTF16FromString(`\??\` + target)
	if err != nil {
		t.Fatal(err)
	}
	display, err := windows.UTF16FromString(target)
	if err != nil {
		t.Fatal(err)
	}
	// Mount-point REPARSE_DATA_BUFFER. Lengths exclude the string
	// terminators, while offsets include the preceding terminator.
	buffer := make([]byte, 16, 16+2*(len(substitute)+len(display)))
	binary.LittleEndian.PutUint32(buffer, windows.IO_REPARSE_TAG_MOUNT_POINT)
	binary.LittleEndian.PutUint16(buffer[4:], uint16(cap(buffer)-8))
	binary.LittleEndian.PutUint16(buffer[10:], uint16(2*(len(substitute)-1)))
	binary.LittleEndian.PutUint16(buffer[12:], uint16(2*len(substitute)))
	binary.LittleEndian.PutUint16(buffer[14:], uint16(2*(len(display)-1)))
	for _, name := range [][]uint16{substitute, display} {
		for _, unit := range name {
			buffer = binary.LittleEndian.AppendUint16(buffer, unit)
		}
	}
	if err := os.Mkdir(link, 0o700); err != nil {
		t.Fatal(err)
	}
	path, err := windows.UTF16PtrFromString(link)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(path, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(h)
	var transferred uint32
	if err := windows.DeviceIoControl(h, windows.FSCTL_SET_REPARSE_POINT,
		&buffer[0], uint32(len(buffer)), nil, 0, &transferred, nil); err != nil {
		t.Fatalf("create NTFS directory junction: %v", err)
	}
}
