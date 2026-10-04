package managesieve

import (
	"bufio"
	"crypto/tls"
	"net"
	"strings"
	"testing"
)

// #3：配了证书但未加密时，CAPABILITY 不通告 SASL，AUTHENTICATE 被拒。
func TestAuthDisabledWithoutTLS(t *testing.T) {
	srv, cli := net.Pipe()
	go handle(srv, nil, &tls.Config{}, false)
	defer cli.Close()
	r := bufio.NewReader(cli)
	w := bufio.NewWriter(cli)
	read := func() string { l, _ := r.ReadString('\n'); return strings.TrimRight(l, "\r\n") }
	send := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }

	// 读到 greeting 结束（以 OK 开头的行）
	capLines := readUntilOK(t, read)
	if strings.Contains(strings.Join(capLines, "\n"), `"SASL"`) {
		t.Fatalf("未加密且有证书时不应通告 SASL: %v", capLines)
	}

	send(`AUTHENTICATE "PLAIN" "AGJvYgBiYXI="`)
	if l := read(); !strings.HasPrefix(l, "NO") || !strings.Contains(l, "STARTTLS") {
		t.Fatalf("未加密 AUTHENTICATE 应被拒: %q", l)
	}
}

func readUntilOK(t *testing.T, read func() string) []string {
	t.Helper()
	var out []string
	for i := 0; i < 20; i++ {
		l := read()
		out = append(out, l)
		if strings.HasPrefix(l, "OK") {
			return out
		}
	}
	t.Fatal("未收到 OK 结束")
	return out
}
