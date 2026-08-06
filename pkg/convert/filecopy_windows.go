//go:build windows

package convert

import (
	"os"
	"syscall"
	"unsafe"
)

var kernel32 = syscall.NewLazyDLL("kernel32.dll")
var procCopyFileW = kernel32.NewProc("CopyFileW")

// filecopy копирует файл src в dst через CopyFileW (быстрее io.Copy на больших файлах).
func filecopy(src, dst string) (int64, error) {
	srcPtr, err := syscall.UTF16PtrFromString(src)
	if err != nil {
		return 0, err
	}
	dstPtr, err := syscall.UTF16PtrFromString(dst)
	if err != nil {
		return 0, err
	}
	ret, _, callErr := procCopyFileW.Call(
		//nolint:gosec // G103: uintptr(unsafe.Pointer(...)) обязателен для вызова win32 CopyFileW
		uintptr(unsafe.Pointer(srcPtr)),
		//nolint:gosec // G103: uintptr(unsafe.Pointer(...)) обязателен для вызова win32 CopyFileW
		uintptr(unsafe.Pointer(dstPtr)),
		0, // failIfExists = false
	)
	if ret == 0 {
		return 0, callErr
	}
	fi, err := os.Stat(src)
	if err != nil {
		return 0, err
	}
	return fi.Size(), nil
}
