package handler

// 管理后台：实测发件中继（连接 → EHLO → STARTTLS → AUTH），把失败原因直接显示给用户。

import (
	"context"
	"crypto/tls"
	"errors"
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
	steps, err := testRelay(r.Context(), c.Host, c.Port, c.User, c.Pass, c.Insecure)
	resp := map[string]any{"ok": err == nil, "steps": steps, "host": c.Host, "port": c.Port}
	if err != nil {
		resp["error"] = err.Error()
		es := err.Error()
		switch {
		case strings.Contains(es, "certificate"):
			resp["hint"] = "证书与中继地址不一致：请确认 relay_host 填的是外部发信服务（如 smtp.gmail.com），而不是自己的域名；确需忽略可勾选「跳过 TLS 证书校验」。"
		case strings.Contains(es, "超时"):
			resp["hint"] = "TCP 连接超时：多为云厂商封禁出站 25/587 或安全组未放行。Gmail 请用 587（STARTTLS）或 465（SSL），并确认服务器能访问外网。"
		case strings.Contains(es, "认证"):
			if strings.Contains(strings.ToLower(c.Host), "gmail") {
				resp["hint"] = "Gmail：请开启两步验证并生成「应用专用密码」，不要直接用 Google 登录密码；发件人地址需为 Gmail 账号或已在 Gmail 中验证的别名。"
			}
		}
	}
	writeJSON(w, 200, resp)
}

func testRelay(ctx context.Context, host, port, user, pass string, insecure bool) ([]string, error) {
	var steps []string
	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if port == "" {
		port = "587"
	}
	addr := net.JoinHostPort(host, port)

	if ips, err := net.LookupHost(host); err == nil {
		steps = append(steps, "DNS 解析成功："+strings.Join(ips, ", "))
	} else {
		return steps, fmt.Errorf("DNS 解析 %s 失败：%w（请检查 relay_host 是否填写正确）", host, err)
	}

	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return steps, fmt.Errorf("TCP 连接 %s 超时：多为云厂商封禁出站端口或安全组未放行。Gmail 请用 587（STARTTLS）或 465（SSL）", addr)
		}
		return steps, fmt.Errorf("TCP 连接 %s 失败：%w", addr, err)
	}
	steps = append(steps, "TCP 连接成功")
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	if port == "465" || port == "8465" {
		conn = tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecure})
		steps = append(steps, "已启用隐式 TLS（465）")
	}
	cl, err := smtp.NewClient(conn, host)
	if err != nil {
		return steps, fmt.Errorf("SMTP 握手失败：%w", err)
	}
	defer cl.Close()
	if ok, _ := cl.Extension("STARTTLS"); ok {
		if err := cl.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, InsecureSkipVerify: insecure}); err != nil {
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
