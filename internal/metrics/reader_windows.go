//go:build windows

package metrics

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

func newReader() reader { return winReader{} }

// winReader reads host and process counters through kernel32. Times come
// back as FILETIME-style 100 ns units; procTicks converts to 10 ms ticks so
// the shared percentage maths (clkTck) applies unchanged.
type winReader struct{}

var (
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	procGetSystemTimes          = kernel32.NewProc("GetSystemTimes")
	procGlobalMemoryStatusEx    = kernel32.NewProc("GlobalMemoryStatusEx")
	procK32GetProcessMemoryInfo = kernel32.NewProc("K32GetProcessMemoryInfo")
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

type processMemoryCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func filetimeToUint64(ft windows.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}

func (winReader) cpu() (busy, total uint64, err error) {
	var idle, kernel, user windows.Filetime
	r, _, callErr := procGetSystemTimes.Call(
		uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kernel)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return 0, 0, fmt.Errorf("metrics: GetSystemTimes: %w", callErr)
	}
	// Kernel time includes idle time.
	total = filetimeToUint64(kernel) + filetimeToUint64(user)
	busy = total - filetimeToUint64(idle)
	return busy, total, nil
}

func (winReader) mem() (total, available int64, err error) {
	var st memoryStatusEx
	st.Length = uint32(unsafe.Sizeof(st))
	r, _, callErr := procGlobalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&st)))
	if r == 0 {
		return 0, 0, fmt.Errorf("metrics: GlobalMemoryStatusEx: %w", callErr)
	}
	return int64(st.TotalPhys), int64(st.AvailPhys), nil //nolint:gosec // physical memory fits in int64
}

// load: Windows keeps no load average; the dashboard shows 0.
func (winReader) load() (float64, error) { return 0, errors.New("metrics: no load average on windows") }

func openProcess(pid int) (windows.Handle, error) {
	return windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // pid validated by caller
}

func (winReader) procTicks(pid int) (uint64, error) {
	h, err := openProcess(pid)
	if err != nil {
		return 0, fmt.Errorf("metrics: open process %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0, fmt.Errorf("metrics: GetProcessTimes %d: %w", pid, err)
	}
	// 100 ns units -> 10 ms ticks (clkTck = 100).
	return (filetimeToUint64(kernel) + filetimeToUint64(user)) / 100_000, nil
}

func (winReader) procRSS(pid int) (int64, error) {
	h, err := openProcess(pid)
	if err != nil {
		return 0, fmt.Errorf("metrics: open process %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	var pmc processMemoryCounters
	pmc.CB = uint32(unsafe.Sizeof(pmc))
	r, _, callErr := procK32GetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.CB))
	if r == 0 {
		return 0, fmt.Errorf("metrics: GetProcessMemoryInfo %d: %w", pid, callErr)
	}
	return int64(pmc.WorkingSetSize), nil //nolint:gosec // working set fits in int64
}
