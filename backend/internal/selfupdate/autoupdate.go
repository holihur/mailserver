package selfupdate

import (
	"context"
	"time"
)

// AutoLoop 周期性检查更新。每轮按 interval() 的间隔查询最新版本：
//   - 有新版本且 enabled() 为 true 时自动安装（安装成功后进程通常会被重启）；
//   - 未开启自动更新时只记录日志。
//
// interval()/enabled() 每轮重新求值，因此后台修改配置可即时生效。
func AutoLoop(ctx context.Context, repo, current string, interval func() time.Duration, enabled func() bool, log func(string, ...any)) {
	if repo == "" {
		repo = DefaultRepo
	}
	for {
		wait := 10 * time.Minute
		if interval != nil {
			if d := interval(); d > 0 {
				wait = d
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		cctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		latest, err := Latest(cctx, repo)
		if err != nil {
			if log != nil {
				log("自动更新：检查失败: %v", err)
			}
			cancel()
			continue
		}
		if !Differs(current, latest) {
			cancel()
			continue
		}
		if enabled == nil || !enabled() {
			if log != nil {
				log("自动更新：发现新版本 %s（当前 %s），自动更新已关闭，跳过", latest, current)
			}
			cancel()
			continue
		}
		if log != nil {
			log("自动更新：发现新版本 %s，开始更新…", latest)
		}
		if err := Run(cctx, Options{Repo: repo, Current: current, Log: log}); err != nil {
			if log != nil {
				log("自动更新失败: %v", err)
			}
		}
		cancel()
		// 若进程未重启（如容器内无 systemd），更新 current 以免反复下载同一版本。
		current = latest
	}
}
