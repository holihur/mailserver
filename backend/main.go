package main

import (
	"compress/gzip"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"mailserver/internal/auth"
	"mailserver/internal/backup"
	"mailserver/internal/blob"
	"mailserver/internal/certstore"
	"mailserver/internal/config"
	"mailserver/internal/dane"
	"mailserver/internal/db"
	"mailserver/internal/deliver"
	"mailserver/internal/dnsserver"
	"mailserver/internal/external"
	"mailserver/internal/handler"
	"mailserver/internal/health"
	"mailserver/internal/imap"
	"mailserver/internal/jmap"
	"mailserver/internal/logging"
	"mailserver/internal/mailqueue"
	"mailserver/internal/managesieve"
	"mailserver/internal/mcp"
	"mailserver/internal/message"
	"mailserver/internal/model"
	"mailserver/internal/pop3"
	"mailserver/internal/ratelimit"
	"mailserver/internal/runtimecfg"
	"mailserver/internal/secret"
	"mailserver/internal/selfupdate"
	mailsmtp "mailserver/internal/smtp"

	"github.com/joho/godotenv"
	"gorm.io/gorm"
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
			fmt.Println("用法: mailserver [version|update|backup|restore]")
			fmt.Println("  (无参数)  启动服务")
			fmt.Println("  version  显示版本")
			fmt.Println("  update   从 GitHub Release 更新到最新版并重启服务")
			fmt.Println("  backup [文件]   备份数据库与数据目录到 tar.gz（默认自动命名）")
			fmt.Println("  restore <文件>  从备份恢复")
			fmt.Println("  migrate-blobs   把存量 base64 附件迁移为 blob")
			fmt.Println("  fix-attachment-sizes  回填存量附件的真实字节数（配额统计用）")
			fmt.Println("  gc-blobs        清理未被引用的 blob 文件")
			return
		case "migrate-blobs":
			_ = godotenv.Load()
			c := config.Load()
			g, err := db.Open(c.DatabaseURL)
			if err != nil {
				log.Fatal(err)
			}
			bs, err := blob.New(filepath.Join(c.DataDir, "blobs"))
			if err != nil {
				log.Fatal(err)
			}
			message.SetBlobStore(bs)
			n, err := message.MigrateAttachments(g)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("已迁移 %d 封邮件的附件到 blob\n", n)
			if n2, err := message.BackfillAttachmentSizes(g); err == nil {
				fmt.Printf("已回填 %d 封邮件的附件大小\n", n2)
			}
			return
		case "fix-attachment-sizes":
			_ = godotenv.Load()
			c := config.Load()
			g, err := db.Open(c.DatabaseURL)
			if err != nil {
				log.Fatal(err)
			}
			bs, err := blob.New(filepath.Join(c.DataDir, "blobs"))
			if err != nil {
				log.Fatal(err)
			}
			message.SetBlobStore(bs)
			n, err := message.BackfillAttachmentSizes(g)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("已回填 %d 封邮件的附件大小\n", n)
			return
		case "gc-blobs":
			_ = godotenv.Load()
			c := config.Load()
			g, err := db.Open(c.DatabaseURL)
			if err != nil {
				log.Fatal(err)
			}
			bs, err := blob.New(filepath.Join(c.DataDir, "blobs"))
			if err != nil {
				log.Fatal(err)
			}
			message.SetBlobStore(bs)
			n, err := message.GCBlobs(g)
			if err != nil {
				log.Fatal(err)
			}
			fmt.Printf("已清理 %d 个孤儿 blob\n", n)
			return
		case "backup":
			_ = godotenv.Load()
			c := config.Load()
			g, err := db.Open(c.DatabaseURL)
			if err != nil {
				log.Fatal(err)
			}
			out := "backup-" + time.Now().Format("20060102-150405") + ".tar.gz"
			if len(os.Args) > 2 {
				out = os.Args[2]
			}
			if err := backup.Create(g, c.DataDir, out, []byte(c.JWTSecret)); err != nil {
				log.Fatal("备份失败: ", err)
			}
			fmt.Println("备份完成:", out)
			return
		case "restore":
			_ = godotenv.Load()
			c := config.Load()
			if len(os.Args) < 3 {
				fmt.Println("用法: mailserver restore <备份文件>")
				return
			}
			g, err := db.Open(c.DatabaseURL)
			if err != nil {
				log.Fatal(err)
			}
			safety, err := backup.Restore(g, c.DataDir, os.Args[2], []byte(c.JWTSecret))
			if err != nil {
				log.Fatal("恢复失败: ", err)
			}
			fmt.Println("恢复完成（恢复前状态已备份到", safety, "）")
			return
		}
	}

	_ = godotenv.Load()
	cfg := config.Load()
	// 日志：写入文件并按大小滚动，同时保留 stdout（Docker / systemd 收集）
	if closeLog, err := logging.Setup(logging.Options{
		File:       cfg.LogFile,
		MaxMB:      cfg.LogMaxMB,
		MaxBackups: cfg.LogMaxBackups,
		MaxAgeDays: cfg.LogMaxAgeDays,
		Compress:   cfg.LogCompress,
		AlsoStdout: true,
	}); err != nil {
		log.Println("日志文件初始化失败，仅输出到 stdout:", err)
	} else {
		defer closeLog()
		if cfg.LogFile != "" {
			log.Printf("日志文件: %s (max %dMB × %d, age %dd, compress=%v)", cfg.LogFile, cfg.LogMaxMB, cfg.LogMaxBackups, cfg.LogMaxAgeDays, cfg.LogCompress)
		}
	}
	auth.SetSecret(cfg.JWTSecret)
	secret.SetKey(cfg.JWTSecret)
	// 出站 DANE：对方 TLSA + DNSSEC 验证通过时强制证书匹配
	dane.Enabled = cfg.DANEEnable
	dane.Resolver = cfg.DANEResolver
	// 附件内容寻址存储（DATA_DIR/blobs）；不可用则退化为 base64
	if bs, err := blob.New(filepath.Join(cfg.DataDir, "blobs")); err != nil {
		log.Println("blob 存储初始化失败，附件仍用 base64:", err)
	} else {
		message.SetBlobStore(bs)
	}
	// SRS：转发时重写信封发件人（用邮件域，保证 SPF 对齐）
	deliver.SRSSecret = []byte(cfg.JWTSecret)
	if cfg.DKIMDomain != "" {
		deliver.SRSAliasDomain = cfg.DKIMDomain
	} else {
		deliver.SRSAliasDomain = cfg.Host
	}
	if cfg.JWTSecret == "dev-secret-change-me-32chars!!" || len(cfg.JWTSecret) < 16 {
		log.Println("⚠️  安全警告：JWT_SECRET 为默认值或过短；它同时用于登录令牌与凭证加密，请设置随机 32 位以上")
	}

	g, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatal(err)
	}
	// 每日 GC 未被引用的 blob
	go func() {
		for {
			time.Sleep(24 * time.Hour)
			if n, err := message.GCBlobs(g); err == nil && n > 0 {
				log.Printf("blob GC 清理 %d 个孤儿文件", n)
			}
		}
	}()

	au := &handler.Auth{DB: g, AdminEmails: cfg.AdminEmails}
	rl, err := ratelimit.New(cfg.RedisURL)
	if err != nil {
		log.Fatal("限流需要 Redis（REDIS_URL）：", err)
	}
	au.RL = rl
	mb := &handler.MailBox{DB: g}
	tb := &handler.TokenBox{DB: g}
	sb := &handler.ScheduledBox{DB: g}
	contacts := &handler.ContactBox{DB: g}
	extBox := &handler.ExternalBox{DB: g}
	folderBox := &handler.FolderBox{DB: g}
	sieveBox := &handler.SieveBox{DB: g}
	proxyBox := &handler.ProxyBox{DB: g}
	totpBox := &handler.TOTPBox{DB: g}
	gdpr := &handler.GDPRBox{DB: g}
	dns := handler.NewDNS(g, cfg.DataDir)

	// 运行时配置（后台可改，DB 持久化，环境变量仅作引导）
	rt := runtimecfg.New(g, cfg)
	// asynq 发信队列（Redis 后端）：入队 + worker + 定时兜底
	mq, err := mailqueue.NewClient(cfg.RedisURL, g, cfg.SendDailyLimit, cfg.SendPerMinute)
	if err != nil {
		log.Fatal("发信队列需要 Redis（REDIS_URL）：", err)
	}
	mb.MQ = mq
	extBox.MQ = mq
	// 新 IP 登录提醒：给用户自己发一封站内信
	handler.NotifyLogin = func(db *gorm.DB, uid uint, email, ip, ua string) {
		var u model.User
		if err := db.First(&u, uid).Error; err != nil {
			return
		}
		body := fmt.Sprintf("您的账号 %s 于 %s 从新 IP 登录：\n\nIP: %s\n设备: %s\n\n如非本人操作，请立即修改密码并退出所有设备。\n",
			email, time.Now().Format(time.RFC3339), ip, ua)
		m := model.Mail{UserID: uid, From: u.Email, To: u.Email, Subject: "新登录提醒", Body: body, Folder: "sent", Read: true, Status: "queued"}
		if err := db.Create(&m).Error; err != nil {
			return
		}
		_ = mq.EnqueueSend(m.ID)
	}
	// 发信 vacation：自动回复，按（用户,发件人）7 天去重
	deliver.VacationHook = func(db *gorm.DB, uid uint, from, subject, text string) {
		if !mq.AllowVacation(uid, from, 7) {
			return
		}
		var u model.User
		if err := db.First(&u, uid).Error; err != nil {
			return
		}
		m := model.Mail{UserID: uid, From: u.Email, To: from, Subject: "自动回复: " + subject, Body: text, Folder: "sent", Read: true, Status: "queued"}
		if err := db.Create(&m).Error; err != nil {
			return
		}
		_ = mq.EnqueueSend(m.ID)
	}
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
		DataDir: cfg.DataDir, MasterKey: []byte(cfg.JWTSecret),
		Version: version, Commit: commit, Date: date, Repo: os.Getenv("MAILSERVER_REPO")}
	hc := health.New(cfg.DataDir)
	hc.Start(20 * time.Second)
	ad.Health = hc
	jmapSrv := &jmap.Server{DB: g, MQ: mq, BlobDir: filepath.Join(cfg.DataDir, "blobs")}
	metricsBox := &handler.MetricsBox{DB: g, Health: hc, Version: version, Commit: commit, Token: cfg.MetricsToken}
	au.RT = rt
	dns.RT = rt
	dns.CertDir = cfg.CertDir
	dns.DNSSECOn = cfg.DNSSECEnable
	dns.AdminEmails = cfg.AdminEmails

	host := rt.MailHost
	maxMsg := int64(cfg.MaxMessageMB) << 20

	go mailsmtp.Serve(":"+cfg.SMTPport, g, maxMsg, cfg.DMARCEnforce, tlsConf)
	go external.Start(g)
	handler.StartAuditRetention(g, rt.AuditRetentionDays)
	handler.StartAuthRetention(g, rt.LoginRetentionDays)
	// 定时备份 + 异地 hook（rclone 等）；目录/间隔/保留份数可在后台「备份」页调整
	go func() {
		for {
			dir := rt.BackupDir()
			if dir == "" {
				dir = cfg.BackupDir
			}
			if dir != "" {
				if out, err := backup.Scheduled(g, cfg.DataDir, dir, rt.BackupKeep(), []byte(cfg.JWTSecret)); err != nil {
					log.Println("定时备份失败:", err)
				} else {
					log.Println("定时备份完成:", out)
					if cfg.BackupHook != "" {
						cmd := exec.Command("sh", "-c", cfg.BackupHook)
						cmd.Env = append(os.Environ(), "BACKUP_FILE="+out)
						if err := cmd.Run(); err != nil {
							log.Println("备份 hook 失败:", err)
						}
					}
				}
			}
			time.Sleep(rt.BackupIntervalHours())
		}
	}()
	go mailsmtp.ServeSubmit(":"+cfg.SubmitPort, host, g, tlsConf, maxMsg)
	go pop3.Serve(":"+cfg.Pop3Port, host, g, tlsConf)
	go imap.Serve(":"+cfg.ImapPort, host, g, tlsConf)
	if cfg.ImapTLSPort != "" {
		go imap.ServeTLS(":"+cfg.ImapTLSPort, host, g, tlsConf)
	}
	if cfg.SubmitTLSPort != "" {
		go mailsmtp.ServeSubmitTLS(":"+cfg.SubmitTLSPort, host, g, tlsConf, maxMsg)
	}
	if cfg.Pop3TLSPort != "" {
		go pop3.ServeTLS(":"+cfg.Pop3TLSPort, host, g, tlsConf)
	}
	if cfg.ManageSievePort != "" {
		go managesieve.Serve(":"+cfg.ManageSievePort, g, tlsConf)
	}

	// 内置权威 DNS（与 API 同一进程/二进制；DNS_ADDR=off 可禁用）
	if cfg.DNSAddr != "" && !strings.EqualFold(cfg.DNSAddr, "off") && !strings.EqualFold(cfg.DNSAddr, "none") {
		go func() {
			if cfg.DNSSECEnable {
				dnsserver.EnableDNSSEC(filepath.Join(cfg.DataDir, "dnssec"), cfg.DNSSECNSEC3)
				if ds := dnsserver.DS(cfg.DKIMDomain); ds != nil {
					log.Printf("DNSSEC 已启用；请到注册商设置 DS: %s", ds.String())
				}
			}
			dnsserver.New(dns.ZonesPath, cfg.NSHost).Start(cfg.DNSAddr)
		}()
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
	mux.HandleFunc("/api/oidc/login", cors(au.OIDCLogin))
	mux.HandleFunc("/api/oidc/callback", cors(au.OIDCCallback))
	mux.HandleFunc("/api/totp", cors(totpBox.Status))
	mux.HandleFunc("/api/totp/setup", cors(totpBox.Setup))
	mux.HandleFunc("/api/totp/enable", cors(totpBox.Enable))
	mux.HandleFunc("/api/totp/disable", cors(totpBox.Disable))
	mux.HandleFunc("/api/gdpr/export", cors(gdpr.Export))
	mux.HandleFunc("/api/gdpr/delete", cors(gdpr.Delete))
	mux.HandleFunc("/api/me", cors(au.Me))
	mux.HandleFunc("/api/me/password", cors(au.ChangePassword))
	mux.HandleFunc("/api/me/logins", cors(au.Logins))
	mux.HandleFunc("/api/me/logout-all", cors(au.LogoutAll))
	mux.HandleFunc("/api/me/sessions", cors(au.Sessions))
	mux.HandleFunc("/api/me/sessions/", cors(au.SessionOne))
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
	mux.HandleFunc("/api/mails/import", cors(mb.Import))
	mux.HandleFunc("/api/mails/unread", cors(mb.Unread))
	mux.HandleFunc("/api/mails/", cors(mb.One))
	mux.HandleFunc("/api/tokens", cors(tb.List))
	mux.HandleFunc("/api/tokens/", cors(tb.One))
	mux.HandleFunc("/api/scheduled", cors(sb.List))
	mux.HandleFunc("/api/scheduled/", cors(sb.One))
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
	mux.HandleFunc("/api/proxy/image", cors(proxyBox.Image))
	// JMAP / MCP：HTTP 层按 IP 限流（复用 Redis），防应用层 DDoS / 撞库
	rlHTTP := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := handler.ClientIP(r)
			if !rl.Allow("http:"+ip, 120, time.Minute) {
				http.Error(w, "too many requests", http.StatusTooManyRequests)
				return
			}
			next(w, r)
		}
	}
	mux.HandleFunc("/mcp", cors(rlHTTP(mcpSrv.Handler)))
	mux.HandleFunc("/jmap", cors(rlHTTP(jmapSrv.Handler)))
	mux.HandleFunc("/jmap/", cors(rlHTTP(jmapSrv.Handler)))
	mux.HandleFunc("/.well-known/jmap", cors(rlHTTP(jmapSrv.Handler)))
	mux.HandleFunc("/metrics", metricsBox.Serve)
	mux.HandleFunc("/api/outbox", cors(mb.Outbox))
	// MTA-STS 策略文件（配 MTA_STS_MODE=testing|enforce 时生效）
	mux.HandleFunc("/.well-known/mta-sts.txt", func(w http.ResponseWriter, r *http.Request) {
		mode := strings.ToLower(strings.TrimSpace(cfg.MtaStsMode))
		if mode != "testing" && mode != "enforce" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "version: STSv1\nmode: %s\nmx: %s\nmax_age: 604800\n", mode, rt.MailHost())
	})
	mux.HandleFunc("/api/dkim", cors(dns.DKIM))
	mux.HandleFunc("/api/domains", cors(dns.Domains))
	mux.HandleFunc("/api/domains/", cors(dns.DomainOne))
	// 管理后台（仅管理员）
	mux.HandleFunc("/api/admin/overview", cors(ad.Overview))
	mux.HandleFunc("/api/admin/audit", cors(ad.AuditLogs))
	mux.HandleFunc("/api/admin/deliverability", cors(dns.Deliverability))
	mux.HandleFunc("/api/admin/domain-check", cors(ad.DomainCheck))
	mux.HandleFunc("/api/admin/deliverability/publish", cors(dns.PublishDeliverability))
	mux.HandleFunc("/api/admin/dnssec", cors(dns.DNSSECInfo))
	mux.HandleFunc("/api/admin/health", cors(ad.HealthStatus))
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
	mux.HandleFunc("/api/admin/backups", cors(ad.Backups))
	mux.HandleFunc("/api/admin/backups/restore", cors(ad.RestoreBackup))
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
		Handler:           securityHeaders(handler.AuditAdmin(g, mux)),
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
