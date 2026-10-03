package handler

// 管理后台：实测发件中继（连接 → EHLO → STARTTLS → AUTH），把失败原因直接显示给用户。

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

// POST /api/admin/relay/test
func (a *Admin) RelayTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	if a.RT == nil {
		writeJSON(w, 500, map[string]any{"ok": false, "error": "运行时配置未初始化"})
		return
	}
	c := a.RT.Relay()
	if strings.TrimSpace(c.Host) == "" {
		writeJSON(w, 200, map[string]any{"ok": false, "error": "未配置发件中继（relay_host 为空）"})
		return
	}
	steps, err := testRelay(r.Context(), c.Host, c.Port, c.User, c.Pass)
	resp := map[string]any{"ok": err == nil, "steps": steps, "host": c.Host, "port": c.Port}
	if err != nil {
		resp["error"] = err.Error()
	}
	writeJSON(w, 200, resp)
}

func testRelay(ctx context.Context, host, port, user, pass string) ([]string, error) {
	var steps []string
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if port == "" {
		port = "587"
	}
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return steps, fmt.Errorf("TCP 连接 %s:%s 失败：%w", host, port, err)
	}
	steps = append(steps, "TCP 连接成功")
	if port == "465" || port == "8465" {
		conn = tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
	}
	cl, err := smtp.NewClient(conn, host)
	if err != nil {
		return steps, fmt.Errorf("SMTP 握手失败：%w", err)
	}
	defer cl.Close()
	if ok, _ := cl.Extension("STARTTLS"); ok {
		if err := cl.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); err != nil {
			return steps, fmt.Errorf("STARTTLS 失败：%w", err)
		}
		steps = append(steps, "STARTTLS 成功")
	} else {
		steps = append(steps, "服务器未提供 STARTTLS")
	}
	if user != "" {
		if ok, _ := cl.Extension("AUTH"); ok {
			if err := cl.Auth(smtp.PlainAuth("", user, pass, host)); err != nil {
				return steps, fmt.Errorf("认证失败（检查用户名/密码）：%w", err)
			}
			steps = append(steps, "AUTH 成功")
		} else {
			steps = append(steps, "服务器未提供 AUTH（可能不需要认证）")
		}
	}
	_ = cl.Quit()
	steps = append(steps, "中继可用")
	return steps, nil
}
