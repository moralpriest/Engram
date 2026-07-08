package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/civilware/tela/logger"
)

// SystemHealth holds the results of preflight checks before starting the daemon.
type SystemHealth struct {
	TimeSynced      bool
	TimeSyncError   string
	DiskSpaceGB     float64 // available space on data dir partition
	HasSpaceForFull bool
	DiskIOBytes     float64 // MB/s write throughput
	DiskIOError     string
	InodeConfig     string
	InodeError      string
	RecommendedMode string // "pruned" or "full"
	Passed          bool   // true if time is synced and enough disk for pruned
}

// checkSystemHealth runs all preflight checks and returns the results.
func checkSystemHealth() SystemHealth {
	var health SystemHealth

	// 1. Time sync check (critical - must pass)
	health.TimeSynced, health.TimeSyncError = checkTimeSync()

	// 2. Disk space check
	health.DiskSpaceGB = checkDiskSpace()
	health.HasSpaceForFull = health.DiskSpaceGB >= 250

	// 3. Disk I/O check
	health.DiskIOBytes, health.DiskIOError = checkDiskIO()

	// 4. Inode configuration (Linux only)
	health.InodeConfig, health.InodeError = checkInodeConfig()

	// 5. Determine recommended mode
	if health.HasSpaceForFull {
		health.RecommendedMode = "full"
	} else {
		health.RecommendedMode = "pruned"
	}

	// Passed = time synced AND enough space for at least pruned node
	// Passed if time synced AND (enough disk OR disk space unknown)
	health.Passed = health.TimeSynced && (health.DiskSpaceGB >= 5 || health.DiskSpaceGB == -1)

	return health
}

// checkTimeSync verifies that the system clock is reasonably accurate.
// Returns true if time appears to be within 60 seconds of real time.
func checkTimeSync() (bool, string) {
	// Method 1: Compare with HTTP Date header
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Head("https://dero.rabidmining.com")
	if err == nil {
		dateHeader := resp.Header.Get("Date")
		resp.Body.Close()
		if dateHeader != "" {
			serverTime, err := time.Parse(time.RFC1123, dateHeader)
			if err == nil {
				localTime := time.Now()
				diff := localTime.Sub(serverTime)
				if diff < 0 {
					diff = -diff
				}
				if diff > 60*time.Second {
					return false, fmt.Sprintf("System time is %.0f seconds off from network time. Please sync your clock.", diff.Seconds())
				}
				return true, ""
			}
		}
	}

	// Method 2: Sanity check - time should be after DERO genesis (2017) and not in the future
	year, month := time.Now().Year(), time.Now().Month()
	if year < 2017 || (year == 2017 && month < time.December) {
		return false, "System time is before DERO blockchain genesis (Dec 2017). Please sync your clock."
	}
	if year > 2027 {
		return false, "System time appears to be in the future. Please sync your clock."
	}

	// Method 3: Linux /proc/uptime drift check
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile("/proc/uptime")
		if err == nil {
			fields := strings.Fields(string(data))
			if len(fields) >= 1 {
				uptime, err := strconv.ParseFloat(fields[0], 64)
				if err == nil && uptime > 86400 { // system up for > 1 day
					// If uptime is > 1 day but clock shows 2017 or earlier, definitely wrong
					if time.Now().Year() < 2024 {
						return false, "System has been running for days but clock shows an old date. NTP likely not syncing."
					}
				}
			}
		}
	}

	return true, ""
}

// checkDiskSpace returns available disk space in GB on the data directory partition.
func checkDiskSpace() float64 {
	dataDir := daemonDataDir()
	parent := filepath.Dir(dataDir)

	logger.Printf("[DiskSpace] Checking disk space for data dir: %s", dataDir)
	logger.Printf("[DiskSpace] Parent directory: %s", parent)

	bytes, err := getDiskSpaceBytes(parent)
	if err != nil {
		logger.Printf("[DiskSpace] Failed to get disk space for %s: %v", parent, err)
	}
	available := bytes
	if available > 0 {
		logger.Printf("[DiskSpace] Available bytes: %d", available)
	}

	if available == 0 {
		logger.Printf("[DiskSpace] Could not determine disk space, returning -1 (unknown)")
		return -1
	}

	result := float64(available) / (1024 * 1024 * 1024)
	logger.Printf("[DiskSpace] Available disk space: %.2f GB", result)
	return result
}

// checkDiskIO measures approximate disk write throughput by writing a temp file.
func checkDiskIO() (float64, string) {
	tmpDir := filepath.Join(os.TempDir(), "engram_disk_test")
	os.MkdirAll(tmpDir, 0755)
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "io_test.tmp")
	size := int64(64 * 1024 * 1024) // 64MB test file

	data := make([]byte, size)
	// Fill with random-ish data to avoid compression
	for i := range data {
		data[i] = byte(i & 0xFF)
	}

	start := time.Now()
	if err := os.WriteFile(tmpFile, data, 0644); err != nil {
		return 0, fmt.Sprintf("Could not test disk I/O: %v", err)
	}
	elapsed := time.Since(start)

	// Clean up
	os.Remove(tmpFile)

	mbps := float64(size) / (1024 * 1024) / elapsed.Seconds()

	if mbps < 10 {
		return mbps, fmt.Sprintf("Disk is slow (%.0f MB/s). Blockchain sync may be very slow.", mbps)
	}
	if mbps < 50 {
		return mbps, fmt.Sprintf("Disk speed is moderate (%.0f MB/s). Sync may take a while.", mbps)
	}

	return mbps, ""
}

// checkInodeConfig checks filesystem inode configuration (Linux only).
func checkInodeConfig() (string, string) {
	if runtime.GOOS != "linux" {
		return "N/A (non-Linux)", ""
	}

	dataDir := daemonDataDir()
	parent := filepath.Dir(dataDir)

	totalInodes, freeInodes, err := getInodeInfo(parent)
	if err != nil {
		return "unknown", fmt.Sprintf("Could not check inode config: %v", err)
	}
	total := fmt.Sprintf("%d total, %d free", totalInodes, freeInodes)

	if totalInodes > 0 && float64(freeInodes)/float64(totalInodes) < 0.01 {
		return total, fmt.Sprintf("Critically low on inodes (%d%% remaining). This may cause chain data corruption.",
			freeInodes*100/totalInodes)
	}

	return total, ""
}

// requireTimeSync checks time sync and returns an error if the system clock
// is out of sync. This is used to prevent the daemon from starting with bad time.
func requireTimeSync() error {
	synced, errMsg := checkTimeSync()
	if !synced {
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}
