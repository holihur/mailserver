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

	"gorm.io/gorm"
)

type DNS struct {
	DB        *gorm.DB
	ZonesPath string // zones.json 输出路径
	Signer    *dkim.Signer
}

func zonesDefault(dbPath string) string {
	if p := os.Getenv("ZONES_PATH"); p != "" {
		return p
	}
	dir := filepath.Dir(dbPath)
	if dir == "" || dir == "." {
		dir = "."
	}
	return filepath.Join(dir, "zones.json")
}

func NewDNS(db *gorm.DB, dbPath string) *DNS {
	d := &DNS{DB: db, ZonesPath: zonesDefault(dbPath)}
	d.export() // 启动即导出，保证 dns/ 有文件可加载
	return d
}

func (d *DNS) uid(w http.ResponseWriter, r *http.Request) (uint, bool) {
	uid, err := auth.UserID(r)
	if err != nil {
		writeJSON(w, 401, map[string]string{"error": "unauthorized"})
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
		defs := []model.DnsRecord{
			{DomainID: dm.ID, Name: "@", Type: "NS", Value: "ns1." + in.Name + ".", TTL: 3600},
			{DomainID: dm.ID, Name: "@", Type: "A", Value: ip, TTL: 600},
			{DomainID: dm.ID, Name: "mail", Type: "A", Value: ip, TTL: 600},
			{DomainID: dm.ID, Name: "ns1", Type: "A", Value: ip, TTL: 3600},
			{DomainID: dm.ID, Name: "@", Type: "MX", Value: "mail." + in.Name + ".", TTL: 3600, Prio: 10},
			{DomainID: dm.ID, Name: "@", Type: "TXT", Value: "v=spf1 mx ~all", TTL: 3600},
			{DomainID: dm.ID, Name: "_dmarc", Type: "TXT", Value: "v=DMARC1; p=none; rua=mailto:postmaster@" + in.Name, TTL: 3600},
			{DomainID: dm.ID, Name: "dkim._domainkey", Type: "TXT", Value: "v=DKIM1; k=rsa; p=PASTE_PUBLIC_KEY_HERE", TTL: 3600},
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
func (d *DNS) export() {
	var ds []model.Domain
	d.DB.Find(&ds)
	out := map[string]any{}
	zones := []any{}
	for _, dm := range ds {
		var recs []model.DnsRecord
		d.DB.Where("domain_id = ?", dm.ID).Find(&recs)
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
	_ = os.WriteFile(d.ZonesPath, b, 0644)
}

// GET /api/dkim -> {ready, domain, selector, name, txt}
// POST /api/dkim {domain} -> 公钥写入该域 selector._domainkey TXT 并导出（一键发布）
func (d *DNS) DKIM(w http.ResponseWriter, r *http.Request) {
	if _, ok := d.uid(w, r); !ok {
		return
	}
	if d.Signer == nil {
		writeJSON(w, 200, map[string]any{"ready": false, "hint": "未配 DKIM_KEY，先生成私钥（见 MAIL_CLIENTS.md）"})
		return
	}
	if r.Method == "GET" {
		writeJSON(w, 200, map[string]any{
			"ready": true, "domain": d.Signer.Domain, "selector": d.Signer.Selector,
			"name": d.Signer.Selector + "._domainkey", "txt": d.Signer.TXT(),
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
			dom = d.Signer.Domain
		}
		var dm model.Domain
		if err := d.DB.Where("name = ?", dom).First(&dm).Error; err != nil {
			writeJSON(w, 404, map[string]string{"error": "域名不存在，先添加域名"})
			return
		}
		recName := d.Signer.Selector + "._domainkey"
		var rec model.DnsRecord
		if err := d.DB.Where("domain_id = ? AND name = ? AND type = ?", dm.ID, recName, "TXT").First(&rec).Error; err == nil {
			d.DB.Model(&rec).Update("value", d.Signer.TXT())
		} else {
			d.DB.Create(&model.DnsRecord{DomainID: dm.ID, Name: recName, Type: "TXT", Value: d.Signer.TXT(), TTL: 3600})
		}
		d.export()
		writeJSON(w, 200, map[string]any{"ok": "true", "name": recName + "." + dom, "txt": d.Signer.TXT()})
		return
	}
	w.WriteHeader(405)
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
