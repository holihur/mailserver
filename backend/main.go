package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"mailserver/internal/auth"
	"mailserver/internal/certstore"
	"mailserver/internal/config"
	"mailserver/internal/db"
	"mailserver/internal/dnsserver"
	"mailserver/internal/handler"
	"mailserver/internal/imap"
	"mailserver/internal/pop3"
	"mailserver/internal/queue"
	"mailserver/internal/runtimecfg"
	"mailserver/internal/secret"
	"mailserver/internal/selfupdate"
	mailsmtp "mailserver/internal/smtp"

	"github.com/joho/godotenv"
)

// 版本信息（GoReleaser 通过 -ldflags -X main.version=... 注入）
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// 子命令：version / update / help；否则启动服务
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "-v", "--version":
			fmt.Printf("mailserver %s (commit %s, built %s)\n", version, commit, date)
			return
		case "update":
			err := selfupdate.Run(context.Background(), selfupdate.Options{
				Repo:    os.Getenv("MAILSERVER_REPO"),
				Current: version,
				Force:   os.Getenv("MAILSERVER_UPDATE_FORCE") == "1",
			})
			if err != nil {
				log.Fatal("更新失败: ", err)
			}
			return
		case "help", "-h", "--help":
			fmt.Println("用法: mailserver [version|update]")
			fmt.Println("  (无参数)  启动服务")
			fmt.Println("  version  显示版本")
			fmt.Println("  update   从 GitHub Release 更新到最新版并重启服务")
			return
		}
	}

	_ = godotenv.Load()
	cfg := config.Load()
	auth.SetSecret(cfg.JWTSecret)
	secret.SetKey(cfg.JWTSecret)

	g, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}

	au := &handler.Auth{DB: g, AdminEmails: cfg.AdminEmails}
	mb := &handler.MailBox{DB: g}
	dns := handler.NewDNS(g, cfg.DataDir)

	// 运行时配置（后台可改，DB 持久化，环境变量仅作引导）
	rt := runtimecfg.New(g, cfg)

	// 动态 TLS 证书：支持后台手动上传或 ACME 自动签发后热生效
	cert := certstore.New(cfg.CertDir)
	_ = cert.Load()
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		cb, cerr := os.ReadFile(cfg.TLSCert)
		kb, kerr := os.ReadFile(cfg.TLSKey)
		if cerr == nil && kerr == nil {
			if err := cert.Set(cb, kb, "env"); err != nil {
				log.Println("tls cert load fail:", err)
			}
		} else {
			log.Println("tls cert read fail, plain only")
		}
	}
	tlsConf := cert.TLSConfig()

	ad := &handler.Admin{DB: g, AdminEmails: cfg.AdminEmails, DNS: dns, RT: rt, Cert: cert, CertDir: cfg.CertDir}
	au.RT = rt
	dns.RT = rt

	host := rt.MailHost

	go mailsmtp.Serve(":"+cfg.SMTPport, g)
	go mailsmtp.ServeSubmit(":"+cfg.SubmitPort, host, g, tlsConf)
	go pop3.Serve(":"+cfg.Pop3Port, host, g, tlsConf)
	go imap.Serve(":"+cfg.ImapPort, host, g, tlsConf)
	if cfg.ImapTLSPort != "" {
		go imap.ServeTLS(":"+cfg.ImapTLSPort, host, g, tlsConf)
	}
	if cfg.SubmitTLSPort != "" {
		go mailsmtp.ServeSubmitTLS(":"+cfg.SubmitTLSPort, host, g, tlsConf)
	}
	if cfg.Pop3TLSPort != "" {
		go pop3.ServeTLS(":"+cfg.Pop3TLSPort, host, g, tlsConf)
	}
	go queue.Start(g, rt)

	// 内置权威 DNS（与 API 同一进程/二进制；DNS_ADDR=off 可禁用）
	if cfg.DNSAddr != "" && !strings.EqualFold(cfg.DNSAddr, "off") && !strings.EqualFold(cfg.DNSAddr, "none") {
		go dnsserver.New(dns.ZonesPath, cfg.NSHost).Start(cfg.DNSAddr)
		log.Println("authoritative dns on", cfg.DNSAddr)
	}

	renewCtx, cancelRenew := context.WithCancel(context.Background())
	defer cancelRenew()
	go ad.AutoRenewLoop(renewCtx)

	mux := http.NewServeMux()
	// CORS（dev 联调）
	cors := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,DELETE,OPTIONS")
			if r.Method == "OPTIONS" {
				w.WriteHeader(204)
				return
			}
			next(w, r)
		}
	}

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/version", cors(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"version":%q,"commit":%q,"date":%q}`, version, commit, date)
	}))
	mux.HandleFunc("/api/register", cors(au.Register))
	mux.HandleFunc("/api/site", cors(au.Site))
	mux.HandleFunc("/api/login", cors(au.Login))
	mux.HandleFunc("/api/me", cors(au.Me))
	mux.HandleFunc("/api/mails", cors(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			mb.List(w, r)
			return
		}
		if r.Method == "POST" {
			mb.Create(w, r)
			return
		}
		w.WriteHeader(405)
	}))
	mux.HandleFunc("/api/mails/batch", cors(mb.Batch))
	mux.HandleFunc("/api/mails/", cors(mb.One))
	mux.HandleFunc("/api/outbox", cors(mb.Outbox))
	mux.HandleFunc("/api/dkim", cors(dns.DKIM))
	mux.HandleFunc("/api/domains", cors(dns.Domains))
	mux.HandleFunc("/api/domains/", cors(dns.DomainOne))
	// 管理后台（仅管理员）
	mux.HandleFunc("/api/admin/overview", cors(ad.Overview))
	mux.HandleFunc("/api/admin/users", cors(ad.Users))
	mux.HandleFunc("/api/admin/users/", cors(ad.UserOne))
	mux.HandleFunc("/api/admin/domains", cors(ad.Domains))
	mux.HandleFunc("/api/admin/providers", cors(ad.Providers))
	mux.HandleFunc("/api/admin/providers/", cors(ad.ProviderOne))
	mux.HandleFunc("/api/admin/settings", cors(ad.Settings))
	mux.HandleFunc("/api/admin/relay/test", cors(ad.RelayTest))
	mux.HandleFunc("/api/admin/tls", cors(ad.TLS))
	mux.HandleFunc("/api/admin/tls/manual", cors(ad.TLSManual))
	mux.HandleFunc("/api/admin/tls/acme", cors(ad.TLSAcme))
	mux.HandleFunc("/api/admin/dkim/generate", cors(ad.DKIMGenerate))
	mux.HandleFunc("/api/admin/dkim/upload", cors(ad.DKIMUpload))

	// 静态托管前端：优先本地 ./web（开发/自定义），否则用编译时嵌入的前端，单二进制即可。
	static := staticHandler()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") {
			w.WriteHeader(404)
			return
		}
		if static != nil {
			static.ServeHTTP(w, r)
			return
		}
		w.Write([]byte("mailserver api ok, see /api/health"))
	})

	fmt.Println("api on :" + cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}

//go:embed all:web
var embeddedWeb embed.FS

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
	if st, err := os.Stat("./web"); err == nil && st.IsDir() {
		return http.FileServer(http.Dir("./web"))
	}
	sub, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		return nil
	}
	return http.FileServer(spaFS{http.FS(sub)})
}
