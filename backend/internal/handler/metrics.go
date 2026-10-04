package handler

// Prometheus 指标：GET /metrics（文本格式 0.0.4）。仅暴露聚合指标，不含个人数据。

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"runtime"
	"strings"

	"mailserver/internal/health"
	"mailserver/internal/model"

	"gorm.io/gorm"
)

type MetricsBox struct {
	DB      *gorm.DB
	Health  *health.Collector
	Version string
	Commit  string
	Token   string // METRICS_TOKEN：非空则要求 Bearer token；为空则仅允许本机
}

// authorized 校验抓取方：配了 token 则比对 Bearer；否则仅允许本机（127.0.0.1/::1）。
func (h *MetricsBox) authorized(r *http.Request) bool {
	if h.Token != "" {
		hdr := r.Header.Get("Authorization")
		if !strings.HasPrefix(hdr, "Bearer ") {
			return false
		}
		tok := strings.TrimSpace(strings.TrimPrefix(hdr, "Bearer "))
		return subtle.ConstantTimeCompare([]byte(tok), []byte(h.Token)) == 1
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (h *MetricsBox) Serve(w http.ResponseWriter, r *http.Request) {
	if !h.authorized(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	var b strings.Builder

	gauge := func(name, help string, v float64) {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n%s %g\n", name, help, name, name, v)
	}

	fmt.Fprintf(&b, "# HELP sweetcorn_build_info Build information\n# TYPE sweetcorn_build_info gauge\n")
	fmt.Fprintf(&b, "sweetcorn_build_info{version=%q,commit=%q} 1\n", h.Version, h.Commit)

	if h.Health != nil {
		s := h.Health.Current()
		gauge("sweetcorn_cpu_percent", "CPU usage percent (0-100)", s.CPU)
		gauge("sweetcorn_memory_percent", "Memory usage percent (0-100)", s.Memory)
		gauge("sweetcorn_disk_percent", "Disk usage percent (0-100)", s.Disk)
		gauge("sweetcorn_memory_bytes", "Memory used bytes", float64(s.MemUsed))
		gauge("sweetcorn_disk_bytes", "Disk used bytes", float64(s.DiskUsed))
	}

	if h.DB != nil {
		var users, mails, pending, unread int64
		h.DB.Model(&model.User{}).Count(&users)
		h.DB.Model(&model.Mail{}).Count(&mails)
		h.DB.Model(&model.Mail{}).Where("folder = ? AND relayed = ?", "sent", false).Count(&pending)
		h.DB.Model(&model.Mail{}).Where(map[string]any{"read": false}).Count(&unread)
		gauge("sweetcorn_users_total", "Total user accounts", float64(users))
		gauge("sweetcorn_mails_total", "Total stored mails", float64(mails))
		gauge("sweetcorn_mails_pending", "Pending outgoing mails", float64(pending))
		gauge("sweetcorn_mails_unread", "Unread mails", float64(unread))
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	gauge("sweetcorn_goroutines", "Number of goroutines", float64(runtime.NumGoroutine()))
	gauge("sweetcorn_heap_bytes", "Go heap allocated bytes", float64(ms.HeapAlloc))
	gauge("sweetcorn_up", "Service is up", 1)

	_, _ = w.Write([]byte(b.String()))
}
