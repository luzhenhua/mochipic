package batch

import (
	"io/fs"
	"os"

	"golang.org/x/sys/windows"
)

func identify(path string, st fs.FileInfo) fileID {
	if !st.Mode().IsRegular() {
		return fileID{}
	}
	f, err := os.Open(path)
	if err != nil {
		return fileID{}
	}
	defer f.Close()
	var info windows.ByHandleFileInformation
	if windows.GetFileInformationByHandle(windows.Handle(f.Fd()), &info) != nil {
		return fileID{}
	}
	return fileID{uint64(info.VolumeSerialNumber), uint64(info.FileIndexHigh)<<32 | uint64(info.FileIndexLow)}
}
