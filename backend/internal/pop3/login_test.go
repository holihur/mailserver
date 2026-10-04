package pop3

import (
	"bufio"
	"crypto/tls"
	"net"
	"strings"
	"testing"
)

// #3：配了证书但未加密时，USER 要求先 STLS。
func TestUserRequiresTLS(t *testing.T) {
	srv, cli := net.Pipe()
	go handle(srv, func() string { return "mail.test.local" }, nil, &tls.Config{}, false)
	defer cli.Close()
	r := bufio.NewReader(cli)
	w := bufio.NewWriter(cli)
	read := func() string { l, _ := r.ReadString('\n'); return strings.TrimRight(l, "\r\n") }
	send := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }

	if !strings.HasPrefix(read(), "+OK") {
		t.Fatal("缺少 greeting")
	}

	send("CAPA")
	var capa []string
	for {
		l := read()
		if l == "." {
			break
		}
		capa = append(capa, l)
	}
	if !strings.Contains(strings.Join(capa, "\n"), "STLS") {
		t.Fatalf("CAPA 应含 STLS: %v", capa)
	}

	send("USER bob")
	if l := read(); !strings.HasPrefix(l, "-ERR") || !strings.Contains(l, "STARTTLS") {
		t.Fatalf("明文 USER 应被拒: %q", l)
	}
}
