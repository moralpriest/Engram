//go:build windows

package main

import "golang.org/x/sys/windows"

// getDiskSpaceBytes returns available bytes on the filesystem containing the given path.
func getDiskSpaceBytes(path string) (int64, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var freeBytes uint64
	if err := windows.GetDiskFreeSpaceEx(pathPtr, &freeBytes, nil, nil); err != nil {
		return 0, err
	}
	return int64(freeBytes), nil
}

// getInodeInfo returns inode info — not applicable on Windows/NTFS.
func getInodeInfo(path string) (totalInodes, freeInodes uint64, err error) {
	return 0, 0, nil
}
