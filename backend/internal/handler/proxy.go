package handler

// 远程图片代理：邮件 HTML 中的远程图片默认被拦截，用户点「显示图片」时经此代理加载。
// 带 SSRF 防护：预解析校验 + 连接时二次校验（防 DNS rebinding）+ 禁止重定向。

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"

	"gorm.io/gorm"
)

type ProxyBox struct{ DB *gorm.DB }

var (
	errBadURL    = errors.New("bad url")
	errForbidden = errors.New("forbidden")
	errUpstream  = errors.New("fetch failed")
	errNotImage  = errors.New("not an image")
)

func (h *ProxyBox) Image(w http.ResponseWriter, r *http.Request) {
	if _, ok := uidOf(h.DB, w, r); !ok {
		return
	}
	raw := r.URL.Query().Get("u")
	ct, data, err := fetchImage(r.Context(), raw, 8<<20)
	if err != nil {
		code := http.StatusBadGateway
		switch {
		case errors.Is(err, errBadURL):
			code = http.StatusBadRequest
		case errors.Is(err, errForbidden):
			code = http.StatusForbidden
		case errors.Is(err, errNotImage):
			code = http.StatusUnsupportedMediaType
		}
		http.Error(w, err.Error(), code)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = w.Write(data)
}

// fetchImage 拉取远程图片，带 SSRF 防护，返回 content-type 与内容。
func fetchImage(ctx context.Context, raw string, limit int64) (string, []byte, error) {
	pu, err := url.Parse(raw)
	if err != nil || (pu.Scheme != "http" && pu.Scheme != "https") {
		return "", nil, errBadURL
	}
	// 预解析校验（快速失败）
	ips, err := net.LookupIP(pu.Hostname())
	if err != nil || len(ips) == 0 {
		return "", nil, errForbidden
	}
	for _, ip := range ips {
		if !isPublicIP(ip) {
			return "", nil, errForbidden
		}
	}

	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		// 连接时二次校验：即使 DNS 在两次解析间变化（rebinding）也拦得住
		Control: func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return errForbidden
			}
			ip := net.ParseIP(host)
			if ip == nil || !isPublicIP(ip) {
				return errForbidden
			}
			return nil
		},
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{DialContext: dialer.DialContext},
		// 不跟随重定向：检查只覆盖最初 URL，302 可跳内网
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	req.Header.Set("User-Agent", "Sweetcorn-ImageProxy")
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, errUpstream
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", nil, errUpstream
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return "", nil, errNotImage
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return "", nil, errUpstream
	}
	return ct, data, nil
}

// isPublicIP 只允许可公网路由的地址，拒绝内网/保留段（含 CGNAT、benchmark、IPv4-mapped）。
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	} else if ip.To16() != nil && strings.Contains(ip.String(), ":") {
		// 拒绝 IPv4-mapped IPv6（::ffff:a.b.c.d）
		if ip.To4() != nil {
			return false
		}
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() ||
		ip.IsMulticast() || ip.IsInterfaceLocalMulticast() {
		return false
	}
	for _, cidr := range blockedCIDRs {
		if cidr.Contains(ip) {
			return false
		}
	}
	return true
}

var blockedCIDRs = func() []*net.IPNet {
	var out []*net.IPNet
	for _, s := range []string{
		"100.64.0.0/10",   // CGNAT
		"192.0.0.0/24",    // IETF protocol assignments
		"192.0.2.0/24",    // TEST-NET-1
		"198.18.0.0/15",   // benchmark
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		"240.0.0.0/4",     // reserved
		"::/128",          // unspecified
		"2001:db8::/32",   // documentation
		"fc00::/7",        // ULA
		"fe80::/10",       // link-local
	} {
		if _, n, err := net.ParseCIDR(s); err == nil {
			out = append(out, n)
		}
	}
	return out
}()
