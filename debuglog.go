package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"
)

const (
	debugLogFileName    = "engram_debug.log"
	debugLogMaxSize     = 2 * 1024 * 1024
	debugLogRetainedLen = 1 * 1024 * 1024
)

var (
	debugLogMu          sync.Mutex
	debugLogFile        atomic.Pointer[os.File]
	debugPipeWriter     atomic.Pointer[os.File]
	debugPipeDone       chan struct{}
	debugLogChan        atomic.Pointer[chan []byte]
	debugOriginalStdout atomic.Pointer[os.File]
	debugOriginalStderr atomic.Pointer[os.File]
)

func getDebugLogPath() string {
	dir := AppPath()
	// On macOS the CWD may be read-only (e.g. when launched from the .app bundle).
	// Fall back to ~/Library/Caches/Engram/ which is always writable.
	if runtime.GOOS == "darwin" {
		cacheDir, err := os.UserCacheDir()
		if err == nil {
			dir = filepath.Join(cacheDir, "Engram")
		}
	}
	return filepath.Join(dir, debugLogFileName)
}

func initDebugLog() error {
	path := getDebugLogPath()

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	if err := truncateDebugLogIfNeeded(path); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}

	r, w, err := os.Pipe()
	if err != nil {
		f.Close()
		return err
	}

	debugLogFile.Store(f)
	debugPipeWriter.Store(w)
	debugPipeDone = make(chan struct{})
	ch := make(chan []byte, 5000)
	debugLogChan.Store(&ch)
	debugOriginalStdout.Store(os.Stdout)
	debugOriginalStderr.Store(os.Stderr)

	os.Stdout = w
	os.Stderr = w

	// Background worker to write logs to disk without blocking the main app
	go func() {
		for data := range ch {
			file := debugLogFile.Load()
			if file != nil {
				_, _ = file.Write(data)
			}

			stdout := debugOriginalStdout.Load()
			if stdout != nil {
				_, _ = stdout.Write(data)
			}
		}
	}()

	go func() {
		defer close(debugPipeDone)
		defer r.Close()
		buf := make([]byte, 16*1024)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				data := make([]byte, n)
				copy(data, buf[:n])

				logChPtr := debugLogChan.Load()
				if logChPtr != nil {
					select {
					case *logChPtr <- data:
					default:
						// Buffer full, dropping logs to prevent app freeze
					}
				}
			}
			if err != nil {
				break
			}
		}
		debugLogMu.Lock()
		logChPtr := debugLogChan.Load()
		if logChPtr != nil {
			close(*logChPtr)
			debugLogChan.Store(nil)
		}
		debugLogMu.Unlock()
	}()

	writeDebugLogHeader()

	return nil
}

func truncateDebugLogIfNeeded(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}

		return err
	}

	if info.Size() <= debugLogMaxSize {
		return nil
	}

	start := info.Size() - debugLogRetainedLen
	if start < 0 {
		start = 0
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return err
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0o600)
}

func writeDebugLogHeader() {
	writeDebugLine("\n=== Engram Debug Log Started ===")
	writeDebugLine("Time: %s", time.Now().Format(time.RFC3339))
	writeDebugLine("Version: %s", versionString)
	writeDebugLine("OS/ARCH: %s/%s", runtime.GOOS, runtime.GOARCH)
	writeDebugLine("================================")
}

func writeCrashLog(reason interface{}) {
	stack := debug.Stack()

	writeDebugLine("\n=== Engram Panic Recovered ===")
	writeDebugLine("Time: %s", time.Now().Format(time.RFC3339))
	writeDebugLine("Panic: %v", reason)
	writeDebugLine("Stack trace:\n%s", string(stack))
	writeDebugLine("==============================")
}

func writeDebugLine(format string, a ...interface{}) {
	line := fmt.Sprintf(format+"\n", a...)
	data := []byte(line)

	logChPtr := debugLogChan.Load()
	if logChPtr != nil {
		select {
		case *logChPtr <- data:
		default:
			// Buffer full, dropping logs to prevent app freeze
		}
	}
}

func closeDebugLog() {
	writer := debugPipeWriter.Swap(nil)
	debugLogMu.Lock()
	done := debugPipeDone
	debugLogMu.Unlock()

	if writer != nil {
		_ = writer.Close()
	}

	if done != nil {
		<-done
	}

	os.Stdout = debugOriginalStdout.Load()
	os.Stderr = debugOriginalStderr.Load()

	file := debugLogFile.Swap(nil)
	if file != nil {
		_ = file.Close()
	}
}
