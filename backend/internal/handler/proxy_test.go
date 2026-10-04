package handler

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"169.254.169.254", false},
		{"100.64.0.1", false}, // CGNAT
		{"198.18.0.1", false}, // benchmark
		{"240.0.0.1", false},  // reserved
		{"::1", false},
		{"::ffff:127.0.0.1", false},
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"2606:4700:4700::1111", true},
	}
	for _, c := range cases {
		if got := isPublicIP(net.ParseIP(c.ip)); got != c.want {
			t.Errorf("isPublicIP(%s)=%v want %v", c.ip, got, c.want)
		}
	}
	if isPublicIP(nil) {
		t.Error("nil 应 false")
	}
}

// #11：指向内网（loopback）的 URL 必须被拒。
func TestFetchImageBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("png"))
	}))
	defer srv.Close()
	if _, _, err := fetchImage(context.Background(), srv.URL, 1<<20); !errors.Is(err, errForbidden) {
		t.Fatalf("loopback 应被拒 forbidden，得到 %v", err)
	}
}

func TestFetchImageBadURL(t *testing.T) {
	for _, u := range []string{"", "ftp://x/y", "not a url"} {
		if _, _, err := fetchImage(context.Background(), u, 1<<20); !errors.Is(err, errBadURL) {
			t.Fatalf("%q 应 bad url，得到 %v", u, err)
		}
	}
}
