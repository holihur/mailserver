package imap

import (
	"bufio"
	"crypto/tls"
	"net"
	"strings"
	"testing"
)

// #3：配了证书但未加密时，CAPABILITY 通告 LOGINDISABLED，LOGIN/AUTHENTICATE 被拒。
func TestLoginDisabledWithoutTLS(t *testing.T) {
	srv, cli := net.Pipe()
	go handle(srv, func() string { return "mail.test.local" }, nil, &tls.Config{}, false)
	defer cli.Close()
	r := bufio.NewReader(cli)
	w := bufio.NewWriter(cli)
	read := func() string { l, _ := r.ReadString('\n'); return strings.TrimRight(l, "\r\n") }
	send := func(s string) { w.WriteString(s + "\r\n"); w.Flush() }

	if !strings.HasPrefix(read(), "* OK") {
		t.Fatal("缺少 greeting")
	}

	send("a CAPABILITY")
	var caps string
	for {
		l := read()
		if strings.HasPrefix(l, "a ") {
			break
		}
		if strings.HasPrefix(l, "* CAPABILITY") {
			caps = l
		}
	}
	if !strings.Contains(caps, "LOGINDISABLED") || !strings.Contains(caps, "STARTTLS") {
		t.Fatalf("CAPABILITY 应含 LOGINDISABLED/STARTTLS: %q", caps)
	}

	send("b LOGIN u p")
	var login string
	for {
		l := read()
		if strings.HasPrefix(l, "b ") {
			login = l
			break
		}
	}
	if !strings.HasPrefix(login, "b NO") || !strings.Contains(login, "PRIVACYREQUIRED") {
		t.Fatalf("明文 LOGIN 应被拒: %q", login)
	}
}
