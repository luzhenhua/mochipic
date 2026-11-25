package batch

import (
	"io/fs"
	"syscall"
)

func identify(_ string, st fs.FileInfo) fileID {
	if s, ok := st.Sys().(*syscall.Stat_t); ok {
		return fileID{uint64(s.Dev), s.Ino}
	}
	return fileID{}
}
