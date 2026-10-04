// Package logging 统一日志输出：标准库 log + lumberjack 文件滚动。
// 同时写入文件与 stdout：文件在 DATA_DIR 下持久化并按大小滚动，
// stdout 保留给 Docker / systemd 收集。
package logging

import (
	"io"
	"log"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Options 日志文件滚动参数。
type Options struct {
	File       string // 日志文件路径；空则仅输出 stdout
	MaxMB      int    // 单个日志文件大小上限（MB）
	MaxBackups int    // 保留的旧日志文件个数
	MaxAgeDays int    // 旧日志文件保留天数（0=不按时间清理）
	Compress   bool   // 是否 gzip 压缩旧日志
	AlsoStdout bool   // 是否同时输出到 stdout
}

// Setup 配置全局标准 log 的输出。返回关闭函数（用于进程退出时 flush）。
// 目录会自动创建；初始化失败时回退为仅 stdout。
func Setup(o Options) (func() error, error) {
	writers := []io.Writer{}
	if o.AlsoStdout {
		writers = append(writers, os.Stdout)
	}

	var lj *lumberjack.Logger
	if o.File != "" {
		if dir := filepath.Dir(o.File); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return func() error { return nil }, err
			}
		}
		lj = &lumberjack.Logger{
			Filename:   o.File,
			MaxSize:    o.MaxMB,
			MaxBackups: o.MaxBackups,
			MaxAge:     o.MaxAgeDays,
			Compress:   o.Compress,
			LocalTime:  true,
		}
		writers = append(writers, lj)
	}

	if len(writers) > 0 {
		log.SetOutput(io.MultiWriter(writers...))
	}
	return func() error {
		if lj != nil {
			return lj.Close()
		}
		return nil
	}, nil
}
