package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sync"
	"time"
)

const (
	debugLogFileName    = "engram_debug.log"
	debugLogMaxSize     = 2 * 1024 * 1024
	debugLogRetainedLen = 1 * 1024 * 1024
)

var (
	debugLogMu          sync.Mutex
	debugLogFile        *os.File
	debugPipeWriter     *os.File
	debugPipeDone       chan struct{}
	debugOriginalStdout = os.Stdout
	debugOriginalStderr = os.Stderr
)

func getDebugLogPath() string {
	return filepath.Join(AppPath(), debugLogFileName)
}

func initDebugLog() error {
	path := getDebugLogPath()

	if err := os.MkdirAll(filepath.Dir(path), os.ModePerm); err != nil {
		return err
	}

	if err := truncateDebugLogIfNeeded(path); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}

	r, w, err := os.Pipe()
	if err != nil {
		f.Close()
		return err
	}

	debugLogMu.Lock()
	debugLogFile = f
	debugPipeWriter = w
	debugPipeDone = make(chan struct{})
	debugLogMu.Unlock()

	os.Stdout = w
	os.Stderr = w

	go func() {
		defer close(debugPipeDone)
		defer r.Close()
		_, _ = io.Copy(io.MultiWriter(debugOriginalStdout, f), r)
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

	return os.WriteFile(path, data, 0o644)
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

	debugLogMu.Lock()
	defer debugLogMu.Unlock()

	if debugLogFile != nil {
		_, _ = debugLogFile.WriteString(line)
		_ = debugLogFile.Sync()
	}

	if debugOriginalStderr != nil {
		_, _ = debugOriginalStderr.WriteString(line)
	}
}

func closeDebugLog() {
	debugLogMu.Lock()
	writer := debugPipeWriter
	done := debugPipeDone
	file := debugLogFile
	debugPipeWriter = nil
	debugPipeDone = nil
	debugLogFile = nil
	debugLogMu.Unlock()

	if writer != nil {
		_ = writer.Close()
	}

	if done != nil {
		<-done
	}

	os.Stdout = debugOriginalStdout
	os.Stderr = debugOriginalStderr

	if file != nil {
		_ = file.Close()
	}
}
