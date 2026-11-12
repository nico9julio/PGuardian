package backup

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

type DiskSpaceInfo struct {
	Path       string  `json:"path"`
	FreeBytes  uint64  `json:"freeBytes"`
	TotalBytes uint64  `json:"totalBytes"`
	FreeGB     float64 `json:"freeGB"`
	TotalGB    float64 `json:"totalGB"`
	UsedPct    float64 `json:"usedPct"`
}

func GetDiskSpace(targetDir string) (DiskSpaceInfo, error) {
	_ = os.MkdirAll(targetDir, 0750)
	abs, err := filepath.Abs(targetDir)
	if err != nil {
		abs = targetDir
	}
	ptr, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return DiskSpaceInfo{}, err
	}

	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &freeBytesAvailable, &totalNumberOfBytes, &totalNumberOfFreeBytes); err != nil {
		return DiskSpaceInfo{}, err
	}

	const gb = float64(1024 * 1024 * 1024)
	usedPct := 0.0
	if totalNumberOfBytes > 0 {
		usedPct = float64(totalNumberOfBytes-freeBytesAvailable) / float64(totalNumberOfBytes) * 100.0
	}
	return DiskSpaceInfo{
		Path:       abs,
		FreeBytes:  freeBytesAvailable,
		TotalBytes: totalNumberOfBytes,
		FreeGB:     float64(freeBytesAvailable) / gb,
		TotalGB:    float64(totalNumberOfBytes) / gb,
		UsedPct:    usedPct,
	}, nil
}
