//go:build linux || darwin

package main

import "syscall"

// getDiskSpaceBytes returns available bytes on the filesystem containing the given path.
func getDiskSpaceBytes(path string) (int64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return int64(stat.Bavail) * int64(stat.Bsize), nil
}

// getInodeInfo returns total and free inode counts for the filesystem.
// Inodes are meaningful on Linux (ext4/XFS) and macOS (APFS).
func getInodeInfo(path string) (totalInodes, freeInodes uint64, err error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	return uint64(stat.Files), uint64(stat.Ffree), nil
}
