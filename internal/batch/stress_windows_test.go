package batch

import (
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func reportStressMemory(t *testing.T) {
	t.Helper()
	var counters struct {
		Size, PageFaults               uint32
		PeakWorkingSet, WorkingSet     uintptr
		PeakPagedPool, PagedPool       uintptr
		PeakNonPagedPool, NonPagedPool uintptr
		Pagefile, PeakPagefile         uintptr
	}
	size := unsafe.Sizeof(counters)
	counters.Size = uint32(size)
	getCounters := windows.NewLazySystemDLL("kernel32.dll").NewProc("K32GetProcessMemoryInfo")
	if ok, _, err := getCounters.Call(uintptr(windows.CurrentProcess()), uintptr(unsafe.Pointer(&counters)), size); ok == 0 {
		t.Fatalf("read Windows batch process memory: %v", err)
	}
	t.Logf("Windows batch test process peak working set: %.1f MiB", float64(counters.PeakWorkingSet)/(1<<20))
}
