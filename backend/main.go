package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"mailserver/internal/auth"
	"mailserver/internal/config"
	"mailserver/internal/db"
	"mailserver/internal/dkim"
	"mailserver/internal/handler"
	"mailserver/internal/imap"
	"mailserver/internal/pop3"
	"mailserver/internal/queue"
	"mailserver/internal/secret"
	mailsmtp "mailserver/internal/smtp"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()
	auth.SetSecret(cfg.JWTSecret)
	secret.SetKey(cfg.JWTSecret)

	g, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatal(err)
	}

	au := &handler.Auth{DB: g, AdminEmails: cfg.AdminEmails}
	mb := &handler.MailBox{DB: g}
	dns := handler.NewDNS(g, cfg.DBPath)
	ad := &handler.Admin{DB: g, AdminEmails: cfg.AdminEmails}
	ad.DNS = dns

	var tlsConf *tls.Config
	if cfg.TLSCert != "" && cfg.TLSKey != "" {
		if ce, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey); err == nil {
			tlsConf = &tls.Config{Certificates: []tls.Certificate{ce}}
		} else {
			log.Println("tls cert load fail, plain only:", err)
		}
	}

	var signer *dkim.Signer
	if cfg.DKIMKey != "" {
		if sg, err := dkim.Load(cfg.DKIMDomain, cfg.DKIMSelector, cfg.DKIMKey); err == nil {
			signer = sg
			log.Println("dkim ready:", sg.Selector+"._domainkey."+sg.Domain)
		} else {
			log.Println("dkim load fail:", err)
		}
	}

	go mailsmtp.Serve(":"+cfg.SMTPport, g)
	go mailsmtp.ServeSubmit(":"+cfg.SubmitPort, cfg.Host, g, tlsConf)
	go pop3.Serve(":"+cfg.Pop3Port, cfg.Host, g, tlsConf)
	go imap.Serve(":"+cfg.ImapPort, cfg.Host, g, tlsConf)
	if cfg.ImapTLSPort != "" {
		go imap.ServeTLS(":"+cfg.ImapTLSPort, cfg.Host, g, tlsConf)
	}
	if cfg.SubmitTLSPort != "" {
		go mailsmtp.ServeSubmitTLS(":"+cfg.SubmitTLSPort, cfg.Host, g, tlsConf)
	}
	if cfg.Pop3TLSPort != "" {
		go pop3.ServeTLS(":"+cfg.Pop3TLSPort, cfg.Host, g, tlsConf)
	}
	dnsH := dns
	dnsH.Signer = signer
	go queue.Start(g, queue.RelayConf{
		Host: cfg.RelayHost, Port: cfg.RelayPort, User: cfg.RelayUser,
		Pass: cfg.RelayPass, From: cfg.RelayFrom, Name: cfg.Host,
	}, signer)

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
	mux.HandleFunc("/api/register", cors(au.Register))
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
	mux.HandleFunc("/api/mails/", cors(mb.One))
	mux.HandleFunc("/api/outbox", cors(mb.Outbox))
	mux.HandleFunc("/api/dkim", cors(dnsH.DKIM))
	mux.HandleFunc("/api/domains", cors(dns.Domains))
	mux.HandleFunc("/api/domains/", cors(dns.DomainOne))
	// 管理后台（仅管理员）
	mux.HandleFunc("/api/admin/overview", cors(ad.Overview))
	mux.HandleFunc("/api/admin/users", cors(ad.Users))
	mux.HandleFunc("/api/admin/users/", cors(ad.UserOne))
	mux.HandleFunc("/api/admin/domains", cors(ad.Domains))
	mux.HandleFunc("/api/admin/providers", cors(ad.Providers))
	mux.HandleFunc("/api/admin/providers/", cors(ad.ProviderOne))

	// 静态托管前端（docker 镜像把 dist 拷到 ./web）
	if _, err := os.Stat("./web"); err == nil {
		mux.Handle("/", http.FileServer(http.Dir("./web")))
	} else {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/api") {
				w.WriteHeader(404)
				return
			}
			w.Write([]byte("mailserver api ok, see /api/health"))
		})
	}

	fmt.Println("api on :" + cfg.Port)
	log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
}
