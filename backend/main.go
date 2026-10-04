package main

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/certstore"
	"mailserver/internal/config"
	"mailserver/internal/db"
	"mailserver/internal/dnsserver"
	"mailserver/internal/external"
	"mailserver/internal/handler"
	"mailserver/internal/imap"
	"mailserver/internal/mailqueue"
	"mailserver/internal/managesieve"
	"mailserver/internal/mcp"
	"mailserver/internal/pop3"
	"mailserver/internal/ratelimit"
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
	if cfg.JWTSecret == "dev-secret-change-me-32chars!!" || len(cfg.JWTSecret) < 16 {
		log.Println("⚠️  安全警告：JWT_SECRET 为默认值或过短；它同时用于登录令牌与凭证加密，请设置随机 32 位以上")
	}

	g, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}

	au := &handler.Auth{DB: g, AdminEmails: cfg.AdminEmails}
	rl, err := ratelimit.New(cfg.RedisURL)
	if err != nil {
		log.Fatal("限流需要 Redis（REDIS_URL）：", err)
	}
	au.RL = rl
	mb := &handler.MailBox{DB: g}
	tb := &handler.TokenBox{DB: g}
	contacts := &handler.ContactBox{DB: g}
	extBox := &handler.ExternalBox{DB: g}
	folderBox := &handler.FolderBox{DB: g}
	sieveBox := &handler.SieveBox{DB: g}
	totpBox := &handler.TOTPBox{DB: g}
	gdpr := &handler.GDPRBox{DB: g}
	dns := handler.NewDNS(g, cfg.DataDir)

	// 运行时配置（后台可改，DB 持久化，环境变量仅作引导）
	rt := runtimecfg.New(g, cfg)
	// asynq 发信队列（Redis 后端）：入队 + worker + 定时兜底
	mq, err := mailqueue.NewClient(cfg.RedisURL)
	if err != nil {
		log.Fatal("发信队列需要 Redis（REDIS_URL）：", err)
	}
	mb.MQ = mq
	defer mq.Close()
	mcpSrv := &mcp.Server{DB: g, MQ: mq}
	if err := mailqueue.Start(cfg.RedisURL, g, rt); err != nil {
		log.Fatal("启动 asynq 失败：", err)
	}
	rb := &handler.RuleBox{DB: g, RT: rt, AdminEmails: cfg.AdminEmails}
	srb := &handler.RuleBox{DB: g, RT: rt, AdminEmails: cfg.AdminEmails, Site: true}
	rbx := &handler.RouteBox{DB: g, RT: rt, AdminEmails: cfg.AdminEmails}
	abx := &handler.AliasBox{DB: g, RT: rt, AdminEmails: cfg.AdminEmails}

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

	ad := &handler.Admin{DB: g, AdminEmails: cfg.AdminEmails, DNS: dns, RT: rt, Cert: cert, CertDir: cfg.CertDir,
		Version: version, Commit: commit, Date: date, Repo: os.Getenv("MAILSERVER_REPO")}
	au.RT = rt
	dns.RT = rt
	dns.AdminEmails = cfg.AdminEmails

	host := rt.MailHost

	go mailsmtp.Serve(":"+cfg.SMTPport, g)
	go external.Start(g)
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
	if cfg.ManageSievePort != "" {
		go managesieve.Serve(":"+cfg.ManageSievePort, g, tlsConf)
	}

	// 内置权威 DNS（与 API 同一进程/二进制；DNS_ADDR=off 可禁用）
	if cfg.DNSAddr != "" && !strings.EqualFold(cfg.DNSAddr, "off") && !strings.EqualFold(cfg.DNSAddr, "none") {
		go dnsserver.New(dns.ZonesPath, cfg.NSHost).Start(cfg.DNSAddr)
		log.Println("authoritative dns on", cfg.DNSAddr)
	}

	renewCtx, cancelRenew := context.WithCancel(context.Background())
	defer cancelRenew()
	go ad.AutoRenewLoop(renewCtx)

	// 自动更新：默认每 10 分钟检查一次；后台可开关自动安装与调整间隔。
	autoCtx, cancelAuto := context.WithCancel(context.Background())
	defer cancelAuto()
	go selfupdate.AutoLoop(autoCtx, os.Getenv("MAILSERVER_REPO"), version, rt.UpdateInterval, rt.AutoUpdate, log.Printf)

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
	mux.HandleFunc("/api/changelog", cors(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"markdown": changelogMD})
	}))
	mux.HandleFunc("/api/register", cors(au.Register))
	mux.HandleFunc("/api/site", cors(au.Site))
	mux.HandleFunc("/api/login", cors(au.Login))
	mux.HandleFunc("/api/login/totp", cors(au.LoginTOTP))
	mux.HandleFunc("/api/totp", cors(totpBox.Status))
	mux.HandleFunc("/api/totp/setup", cors(totpBox.Setup))
	mux.HandleFunc("/api/totp/enable", cors(totpBox.Enable))
	mux.HandleFunc("/api/totp/disable", cors(totpBox.Disable))
	mux.HandleFunc("/api/gdpr/export", cors(gdpr.Export))
	mux.HandleFunc("/api/gdpr/delete", cors(gdpr.Delete))
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
	mux.HandleFunc("/api/mails/unread", cors(mb.Unread))
	mux.HandleFunc("/api/mails/", cors(mb.One))
	mux.HandleFunc("/api/tokens", cors(tb.List))
	mux.HandleFunc("/api/tokens/", cors(tb.One))
	mux.HandleFunc("/api/rules", cors(rb.List))
	mux.HandleFunc("/api/rules/test", cors(rb.Test))
	mux.HandleFunc("/api/rules/", cors(rb.One))
	mux.HandleFunc("/api/contacts", cors(contacts.List))
	mux.HandleFunc("/api/contacts/", cors(contacts.One))
	mux.HandleFunc("/api/directory", cors(contacts.Directory))
	mux.HandleFunc("/api/external", cors(extBox.List))
	mux.HandleFunc("/api/external/", cors(extBox.One))
	mux.HandleFunc("/api/folders", cors(folderBox.List))
	mux.HandleFunc("/api/folders/", cors(folderBox.One))
	mux.HandleFunc("/api/sieve", cors(sieveBox.List))
	mux.HandleFunc("/api/sieve/check", cors(sieveBox.Check))
	mux.HandleFunc("/api/sieve/", cors(sieveBox.One))
	mux.HandleFunc("/mcp", cors(mcpSrv.Handler))
	mux.HandleFunc("/api/outbox", cors(mb.Outbox))
	mux.HandleFunc("/api/dkim", cors(dns.DKIM))
	mux.HandleFunc("/api/domains", cors(dns.Domains))
	mux.HandleFunc("/api/domains/", cors(dns.DomainOne))
	// 管理后台（仅管理员）
	mux.HandleFunc("/api/admin/overview", cors(ad.Overview))
	mux.HandleFunc("/api/admin/about", cors(ad.About))
	mux.HandleFunc("/api/admin/update/check", cors(ad.UpdateCheck))
	mux.HandleFunc("/api/admin/update", cors(ad.Update))
	mux.HandleFunc("/api/admin/users", cors(ad.Users))
	mux.HandleFunc("/api/admin/users/", cors(ad.UserOne))
	mux.HandleFunc("/api/admin/rules", cors(srb.List))
	mux.HandleFunc("/api/admin/rules/test", cors(srb.Test))
	mux.HandleFunc("/api/admin/rules/", cors(srb.One))
	mux.HandleFunc("/api/admin/routes", cors(rbx.List))
	mux.HandleFunc("/api/admin/routes/test", cors(rbx.Test))
	mux.HandleFunc("/api/admin/routes/", cors(rbx.One))
	mux.HandleFunc("/api/admin/aliases", cors(abx.List))
	mux.HandleFunc("/api/admin/aliases/", cors(abx.One))
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
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           securityHeaders(mux),
		ReadHeaderTimeout: 15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Fatal(srv.ListenAndServe())
}

// securityHeaders 加常见安全响应头（防 MIME 探测、点击劫持、XSS）。
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; "+
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
	if st, err := os.Stat("./web"); err == nil && st.IsDir() {
		return http.FileServer(http.Dir("./web"))
	}
	sub, err := fs.Sub(embeddedWeb, "web")
	if err != nil {
		return nil
	}
	return http.FileServer(spaFS{http.FS(sub)})
}
