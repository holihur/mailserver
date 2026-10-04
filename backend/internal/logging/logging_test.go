package logging

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 恢复全局 log 输出，避免影响其它测试。
func preserveLogger(t *testing.T) {
	t.Helper()
	w := log.Writer()
	f := log.Flags()
	t.Cleanup(func() { log.SetOutput(w); log.SetFlags(f) })
}

func TestSetupWritesFile(t *testing.T) {
	preserveLogger(t)
	file := filepath.Join(t.TempDir(), "nested", "app.log")
	closeFn, err := Setup(Options{File: file, MaxMB: 1, MaxBackups: 2, MaxAgeDays: 0, Compress: false})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	log.Println("hello-rotation-test")
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(b), "hello-rotation-test") {
		t.Fatalf("log file missing message: %q", string(b))
	}
}

func TestSetupRotates(t *testing.T) {
	preserveLogger(t)
	dir := t.TempDir()
	file := filepath.Join(dir, "app.log")
	closeFn, err := Setup(Options{File: file, MaxMB: 1, MaxBackups: 3, MaxAgeDays: 0, Compress: false})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	// 多条小日志累加超过 1MB，触发滚动（lumberjack 会拒绝单条超过上限的写入）
	chunk := strings.Repeat("x", 600)
	for i := 0; i < 4000; i++ {
		log.Println(chunk)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected rotated files, got %d entries", len(entries))
	}
}

func TestSetupStdoutOnly(t *testing.T) {
	preserveLogger(t)
	closeFn, err := Setup(Options{File: "", AlsoStdout: true})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}
}
