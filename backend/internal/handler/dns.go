package handler

// 域名 + 解析记录管理（自托管权威 DNS 的控制面）。
// 数据存 SQLite，变更后导出 zones.json 给 dns/ 服务加载（文件监听，无需重启）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mailserver/internal/auth"
	"mailserver/internal/dkim"
	"mailserver/internal/model"
	"mailserver/internal/runtimecfg"

	"gorm.io/gorm"
)

type DNS struct {
	DB        *gorm.DB
	ZonesPath string // zones.json 输出路径
	RT        *runtimecfg.Store
}

// signer 返回当前 DKIM 签名器（可能为 nil）。
func (d *DNS) signer() *dkim.Signer {
	if d.RT == nil {
		return nil
	}
	return d.RT.Signer()
}

func zonesDefault(dataDir string) string {
	if p := os.Getenv("ZONES_PATH"); p != "" {
		return p
	}
	if dataDir == "" {
		dataDir = "."
	}
	return filepath.Join(dataDir, "zones.json")
}

func NewDNS(db *gorm.DB, dataDir string) *DNS {
	d := &DNS{DB: db, ZonesPath: zonesDefault(dataDir)}
	d.export() // 启动即导出，保证 dns/ 有文件可加载
	return d
}

func (d *DNS) uid(w http.ResponseWriter, r *http.Request) (uint, bool) {
	uid, err := auth.UserID(r)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return 0, false
	}
	var u model.User
	if err := d.DB.Select("id", "disabled").First(&u, uid).Error; err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
		return 0, false
	}
	if u.Disabled {
		writeJSON(w, 403, map[string]string{"error": "账号已禁用"})
		return 0, false
	}
	return uid, true
}

var validTypes = map[string]bool{
	"A": true, "AAAA": true, "MX": true, "TXT": true,
	"CNAME": true, "NS": true, "SRV": true, "CAA": true,
}

// POST /api/domains {name, ip} -> 自动配齐 mail 所需记录
// GET  /api/domains -> 列表
func (d *DNS) Domains(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.uid(w, r); !ok {
		return
	}
	if r.Method == "GET" {
		var ds []model.Domain
		d.DB.Order("id").Find(&ds)
		if ds == nil {
			ds = []model.Domain{}
		}
		writeJSON(w, 200, ds)
		return
	}
	if r.Method == "POST" {
		var in struct {
			Name string `json:"name"`
			IP   string `json:"ip"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		in.Name = strings.ToLower(strings.Trim(strings.TrimSpace(in.Name), "."))
		if in.Name == "" || strings.Contains(in.Name, "/") {
			writeJSON(w, 400, map[string]string{"error": "域名非法"})
			return
		}
		dm := model.Domain{Name: in.Name}
		if err := d.DB.Create(&dm).Error; err != nil {
			writeJSON(w, 409, map[string]string{"error": "域名已存在"})
			return
		}
		ip := strings.TrimSpace(in.IP)
		if ip == "" {
			ip = "127.0.0.1" // 占位，前端会提示改成公网 IP
		}
		// DKIM：签名域与当前域名一致时直接填入真实公钥，否则留占位
		selector, dkimVal := "dkim", "v=DKIM1; k=rsa; p=PASTE_PUBLIC_KEY_HERE"
		if sg := d.signer(); sg != nil && strings.EqualFold(sg.Domain, in.Name) {
			selector = sg.Selector
			if t := sg.TXT(); t != "" {
				dkimVal = t
			}
		}
		defs := []model.DnsRecord{
			{DomainID: dm.ID, Name: "@", Type: "NS", Value: "ns1." + in.Name + ".", TTL: 3600},
			{DomainID: dm.ID, Name: "@", Type: "A", Value: ip, TTL: 600},
			{DomainID: dm.ID, Name: "mail", Type: "A", Value: ip, TTL: 600},
			{DomainID: dm.ID, Name: "ns1", Type: "A", Value: ip, TTL: 3600},
			{DomainID: dm.ID, Name: "@", Type: "MX", Value: "mail." + in.Name + ".", TTL: 3600, Prio: 10},
			{DomainID: dm.ID, Name: "@", Type: "TXT", Value: "v=spf1 mx ~all", TTL: 3600},
			{DomainID: dm.ID, Name: "_dmarc", Type: "TXT", Value: "v=DMARC1; p=none; rua=mailto:postmaster@" + in.Name, TTL: 3600},
			{DomainID: dm.ID, Name: selector + "._domainkey", Type: "TXT", Value: dkimVal, TTL: 3600},
		}
		for _, rec := range defs {
			d.DB.Create(&rec)
		}
		d.export()
		writeJSON(w, 201, dm)
		return
	}
	w.WriteHeader(405)
}

// /api/domains/{id} /records /zone 的统一分发
func (d *DNS) DomainOne(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.uid(w, r); !ok {
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/domains/")
	parts := strings.Split(rest, "/")
	id64, _ := strconv.ParseUint(parts[0], 10, 32)
	var dm model.Domain
	if err := d.DB.First(&dm, uint(id64)).Error; err != nil {
		writeJSON(w, 404, map[string]string{"error": "domain not found"})
		return
	}
	// GET /api/domains/{id} -> 域名 + 记录
	if len(parts) == 1 {
		switch r.Method {
		case "GET":
			var recs []model.DnsRecord
			d.DB.Where("domain_id = ?", dm.ID).Order("name, type").Find(&recs)
			if recs == nil {
				recs = []model.DnsRecord{}
			}
			writeJSON(w, 200, map[string]any{"domain": dm, "records": recs})
		case "DELETE":
			d.DB.Where("domain_id = ?", dm.ID).Delete(&model.DnsRecord{})
			d.DB.Delete(&dm)
			d.export()
			writeJSON(w, 200, map[string]string{"ok": "true"})
		default:
			w.WriteHeader(405)
		}
		return
	}
	if parts[1] == "zone" && r.Method == "GET" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(d.zoneText(dm)))
		return
	}
	if parts[1] == "records" {
		// POST /api/domains/{id}/records
		if len(parts) == 2 && r.Method == "POST" {
			var in model.DnsRecord
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil {
				writeJSON(w, 400, map[string]string{"error": "bad body"})
				return
			}
			in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
			if !validTypes[in.Type] {
				writeJSON(w, 400, map[string]string{"error": "不支持的 Type"})
				return
			}
			in.ID = 0
			in.DomainID = dm.ID
			if in.TTL <= 0 {
				in.TTL = 600
			}
			d.DB.Create(&in)
			d.export()
			writeJSON(w, 201, in)
			return
		}
		// PATCH/DELETE /api/domains/{id}/records/{rid}
		if len(parts) == 3 {
			rid64, _ := strconv.ParseUint(parts[2], 10, 32)
			var rec model.DnsRecord
			if err := d.DB.Where("id = ? AND domain_id = ?", uint(rid64), dm.ID).First(&rec).Error; err != nil {
				writeJSON(w, 404, map[string]string{"error": "record not found"})
				return
			}
			if r.Method == "DELETE" {
				d.DB.Delete(&rec)
				d.export()
				writeJSON(w, 200, map[string]string{"ok": "true"})
				return
			}
			if r.Method == "PATCH" {
				var in map[string]any
				json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in)
				allow := []string{"name", "type", "value", "ttl", "prio"}
				upd := map[string]any{}
				for _, k := range allow {
					if v, ok := in[k]; ok {
						upd[k] = v
					}
				}
				if t, ok := upd["type"].(string); ok && !validTypes[strings.ToUpper(t)] {
					writeJSON(w, 400, map[string]string{"error": "不支持的 Type"})
					return
				}
				if len(upd) > 0 {
					d.DB.Model(&rec).Updates(upd)
				}
				d.export()
				d.DB.Where("id = ? AND domain_id = ?", uint(rid64), dm.ID).First(&rec)
				writeJSON(w, 200, rec)
				return
			}
		}
	}
	w.WriteHeader(404)
}

func fqdn(name, domain string) string {
	if name == "@" || name == "" {
		return domain + "."
	}
	if strings.HasSuffix(name, ".") {
		return name
	}
	return name + "." + domain + "."
}

// 导出给自研 dns 服务的 zones.json
func (d *DNS) export() { ExportZones(d.DB, d.ZonesPath) }

// ExportZones 把全部托管域名导出为 zones.json（dns/ 服务文件热加载）。
func ExportZones(db *gorm.DB, path string) {
	var ds []model.Domain
	db.Find(&ds)
	out := map[string]any{}
	zones := []any{}
	for _, dm := range ds {
		var recs []model.DnsRecord
		db.Where("domain_id = ?", dm.ID).Find(&recs)
		var rs []any
		for _, r := range recs {
			rs = append(rs, map[string]any{
				"name": r.Name, "type": r.Type, "value": r.Value, "ttl": r.TTL, "prio": r.Prio,
			})
		}
		if rs == nil {
			rs = []any{}
		}
		zones = append(zones, map[string]any{"domain": dm.Name, "records": rs})
	}
	out["zones"] = zones
	b, _ := json.MarshalIndent(out, "", "  ")
	_ = os.WriteFile(path, b, 0644)
}

// BuildMailRecords 生成发布到第三方 DNS 的邮件相关记录（不含自托管 NS/glue）。
// mailHost 为空时用 mail.<domain>；dkimTXT 为空时用占位公钥。
func BuildMailRecords(domain, ip, mailHost, selector, dkimTXT string) []model.DnsRecord {
	domain = strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
	if selector == "" {
		selector = "dkim"
	}
	if mailHost == "" {
		mailHost = "mail." + domain
	}
	mailHost = strings.Trim(strings.TrimSpace(mailHost), ".")
	if dkimTXT == "" {
		dkimTXT = "v=DKIM1; k=rsa; p=PASTE_PUBLIC_KEY_HERE"
	}
	return []model.DnsRecord{
		{Name: "mail", Type: "A", Value: ip, TTL: 600},
		{Name: "@", Type: "MX", Value: mailHost, TTL: 3600, Prio: 10},
		{Name: "@", Type: "TXT", Value: "v=spf1 mx ~all", TTL: 3600},
		{Name: "_dmarc", Type: "TXT", Value: "v=DMARC1; p=none; rua=mailto:postmaster@" + domain, TTL: 3600},
		{Name: selector + "._domainkey", Type: "TXT", Value: dkimTXT, TTL: 3600},
	}
}

// UpsertDomainWithRecords 确保本地存在该域名及其记录（第三方 DNS 一键配置时同步一份供账号/展示用），
// 同名同类型记录存在则更新，最后刷新 zones.json。
func (d *DNS) UpsertDomainWithRecords(domain string, records []model.DnsRecord) (*model.Domain, error) {
	domain = strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
	if domain == "" || strings.ContainsAny(domain, "/ ") {
		return nil, fmt.Errorf("域名非法")
	}
	var dm model.Domain
	if err := d.DB.Where("LOWER(name) = ?", domain).First(&dm).Error; err != nil {
		dm = model.Domain{Name: domain}
		if err := d.DB.Create(&dm).Error; err != nil {
			return nil, err
		}
	}
	for _, r := range records {
		r.ID = 0
		r.DomainID = dm.ID
		var ex model.DnsRecord
		if err := d.DB.Where("domain_id = ? AND name = ? AND type = ?", dm.ID, r.Name, r.Type).First(&ex).Error; err == nil {
			d.DB.Model(&ex).Updates(map[string]any{"value": r.Value, "ttl": r.TTL, "prio": r.Prio})
		} else {
			d.DB.Create(&r)
		}
	}
	d.export()
	return &dm, nil
}

// GET /api/dkim -> {ready, domain, selector, name, txt}
// POST /api/dkim {domain} -> 公钥写入该域 selector._domainkey TXT 并导出（一键发布）
func (d *DNS) DKIM(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.uid(w, r); !ok {
		return
	}
	if d.signer() == nil {
		writeJSON(w, 200, map[string]any{"ready": false, "hint": "尚未生成 DKIM 密钥，可在管理后台一键生成"})
		return
	}
	sg := d.signer()
	if r.Method == "GET" {
		writeJSON(w, 200, map[string]any{
			"ready": true, "domain": sg.Domain, "selector": sg.Selector,
			"name": sg.Selector + "._domainkey", "txt": sg.TXT(),
		})
		return
	}
	if r.Method == "POST" {
		var in struct {
			Domain string `json:"domain"`
		}
		json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in)
		dom := strings.ToLower(strings.Trim(strings.TrimSpace(in.Domain), "."))
		if dom == "" {
			dom = sg.Domain
		}
		var dm model.Domain
		if err := d.DB.Where("name = ?", dom).First(&dm).Error; err != nil {
			writeJSON(w, 404, map[string]string{"error": "域名不存在，先添加域名"})
			return
		}
		recName := sg.Selector + "._domainkey"
		var rec model.DnsRecord
		if err := d.DB.Where("domain_id = ? AND name = ? AND type = ?", dm.ID, recName, "TXT").First(&rec).Error; err == nil {
			d.DB.Model(&rec).Update("value", sg.TXT())
		} else {
			d.DB.Create(&model.DnsRecord{DomainID: dm.ID, Name: recName, Type: "TXT", Value: sg.TXT(), TTL: 3600})
		}
		d.export()
		writeJSON(w, 200, map[string]any{"ok": "true", "name": recName + "." + dom, "txt": sg.TXT()})
		return
	}
	w.WriteHeader(405)
}

// publishDKIM 把当前签名器公钥写入该域的 selector._domainkey TXT 并导出。
func (d *DNS) publishDKIM(domain string) (name, txt string, err error) {
	sg := d.signer()
	if sg == nil {
		return "", "", fmt.Errorf("未配置 DKIM 密钥")
	}
	name = sg.Selector + "._domainkey"
	txt = sg.TXT()
	var dm model.Domain
	if err := d.DB.Where("name = ?", domain).First(&dm).Error; err != nil {
		return name, txt, fmt.Errorf("域名 %s 不存在，先添加域名", domain)
	}
	var rec model.DnsRecord
	if err := d.DB.Where("domain_id = ? AND name = ? AND type = ?", dm.ID, name, "TXT").First(&rec).Error; err == nil {
		d.DB.Model(&rec).Update("value", txt)
	} else {
		d.DB.Create(&model.DnsRecord{DomainID: dm.ID, Name: name, Type: "TXT", Value: txt, TTL: 3600})
	}
	d.export()
	return name, txt, nil
}

// 预览 BIND 风格 zone（调试用，权威应答以 dns/ 服务为准）
func (d *DNS) zoneText(dm model.Domain) string {
	var recs []model.DnsRecord
	d.DB.Where("domain_id = ?", dm.ID).Order("name, type").Find(&recs)
	var sb strings.Builder
	fmt.Fprintf(&sb, "$ORIGIN %s.\n@ 3600 IN SOA ns1.%s. hostmaster.%s. (1 7200 3600 1209600 300)\n", dm.Name, dm.Name, dm.Name)
	for _, r := range recs {
		n := r.Name
		if n == "@" {
			n = "@"
		}
		val := r.Value
		if r.Type == "MX" {
			fmt.Fprintf(&sb, "%s %d IN MX %d %s\n", n, r.TTL, r.Prio, val)
		} else if r.Type == "TXT" {
			if len(val) > 200 {
				// 长 TXT（如 DKIM）切多段：TXT ("aaa" "bbb")
				var parts []string
				for len(val) > 0 {
					p := 200
					if len(val) < p {
						p = len(val)
					}
					parts = append(parts, fmt.Sprintf("\"%s\"", val[:p]))
					val = val[p:]
				}
				fmt.Fprintf(&sb, "%s %d IN TXT (%s)\n", n, r.TTL, strings.Join(parts, " "))
			} else {
				fmt.Fprintf(&sb, "%s %d IN TXT \"%s\"\n", n, r.TTL, val)
			}
		} else {
			fmt.Fprintf(&sb, "%s %d IN %s %s\n", n, r.TTL, r.Type, val)
		}
	}
	_ = fqdn
	return sb.String()
}
