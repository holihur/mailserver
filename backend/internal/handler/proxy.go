package handler

// 远程图片代理：邮件 HTML 中的远程图片默认被拦截，用户点「显示图片」时经此代理加载，
// 既满足 CSP img-src 'self'，又避免暴露用户 IP。带 SSRF 防护（拒绝内网/环回）。

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gorm.io/gorm"
)

type ProxyBox struct{ DB *gorm.DB }

func (h *ProxyBox) Image(w http.ResponseWriter, r *http.Request) {
	if _, ok := uidOf(h.DB, w, r); !ok {
		return
	}
	raw := r.URL.Query().Get("u")
	pu, err := url.Parse(raw)
	if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
		http.Error(w, "bad url", http.StatusBadRequest)
		return
	}
	ips, err := net.LookupIP(pu.Hostname())
	if err != nil || len(ips) == 0 {
		http.Error(w, "resolve failed", http.StatusBadRequest)
		return
	}
	for _, ip := range ips {
		if isPrivateIP(ip) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	req.Header.Set("User-Agent", "Sweetcorn-ImageProxy")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "fetch failed", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		http.Error(w, "upstream error", http.StatusBadGateway)
		return
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		http.Error(w, "not an image", http.StatusUnsupportedMediaType)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = io.Copy(w, io.LimitReader(resp.Body, 8<<20))
}

func isPrivateIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
