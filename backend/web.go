package main

// 静态资源服务与 HTTP 中间件（从 main.go 拆出）。

import (
	"compress/gzip"
	"embed"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// securityHeaders 加常见安全响应头（防 MIME 探测、点击劫持、XSS）。
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data: blob:; style-src 'self' 'unsafe-inline'; "+
				"script-src 'self' 'unsafe-inline'; connect-src 'self' https://api.ipify.org; "+
				"object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

//go:embed all:web
var embeddedWeb embed.FS

//go:embed CHANGELOG.md
var changelogMD string

// spaFS 在文件不存在时回退到 index.html（前端里实际用的是 hash 路由）。
type spaFS struct{ fs http.FileSystem }

func (s spaFS) Open(name string) (http.File, error) {
	f, err := s.fs.Open(name)
	if err != nil {
		return s.fs.Open("/index.html")
	}
	return f, nil
}

func staticHandler() http.Handler {
	var h http.Handler
	if st, err := os.Stat("./web"); err == nil && st.IsDir() {
		h = http.FileServer(http.Dir("./web"))
	} else {
		sub, err := fs.Sub(embeddedWeb, "web")
		if err != nil {
			return nil
		}
		h = http.FileServer(spaFS{http.FS(sub)})
	}
	return gzipCache(h)
}

// gzipCache 对静态资源做 gzip 压缩与缓存头（hashed 资源 immutable，HTML 不缓存）。
func gzipCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		switch {
		case strings.HasPrefix(p, "/assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case p == "/" || strings.HasSuffix(p, ".html") || !strings.Contains(path.Base(p), "."):
			w.Header().Set("Cache-Control", "no-cache")
		}
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") && r.Header.Get("Range") == "" && compressible(p) {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Add("Vary", "Accept-Encoding")
			gz := gzip.NewWriter(w)
			defer gz.Close()
			next.ServeHTTP(&gzipWriter{ResponseWriter: w, gz: gz}, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type gzipWriter struct {
	http.ResponseWriter
	gz *gzip.Writer
}

func (g *gzipWriter) WriteHeader(code int) {
	g.Header().Del("Content-Length")
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipWriter) Write(b []byte) (int, error) { return g.gz.Write(b) }

func compressible(p string) bool {
	switch path.Ext(p) {
	case ".js", ".mjs", ".css", ".html", ".svg", ".json", ".webmanifest", ".txt", ".map", ".xml":
		return true
	}
	// 无扩展名（如 /）通常是 index.html
	return !strings.Contains(path.Base(p), ".")
}
