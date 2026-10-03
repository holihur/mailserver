package provider

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ---- 阿里云云解析 DNS (Alidns) RPC API ----
// 凭证：access_key_id + access_key_secret。
// 文档：https://help.aliyun.com/zh/dns/api-alidns-2015-01-09/

type aliyun struct {
	keyID  string
	secret string
	http   *http.Client
	base   string
}

func newAliyun(creds map[string]string) (*aliyun, error) {
	a := &aliyun{
		keyID:  strings.TrimSpace(creds["access_key_id"]),
		secret: strings.TrimSpace(creds["access_key_secret"]),
		http:   &http.Client{Timeout: 20 * time.Second},
		base:   "https://alidns.aliyuncs.com/",
	}
	if a.keyID == "" || a.secret == "" {
		return nil, fmt.Errorf("aliyun: 需要 AccessKey ID 和 AccessKey Secret")
	}
	return a, nil
}

// percentEncode 按阿里云 RPC 规范编码（RFC3986，空格 %20）。
func percentEncode(s string) string {
	const hexUpper = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			b.WriteByte('%')
			b.WriteByte(hexUpper[c>>4])
			b.WriteByte(hexUpper[c&0x0f])
		}
	}
	return b.String()
}

func aliyunNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// aliyunSign 计算 HMAC-SHA1 签名。
func aliyunSign(params map[string]string, secret string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, percentEncode(k)+"="+percentEncode(params[k]))
	}
	cqs := strings.Join(parts, "&")
	stringToSign := "GET&%2F&" + percentEncode(cqs)
	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func (a *aliyun) call(ctx context.Context, action string, params map[string]string, out any) error {
	p := map[string]string{
		"Format":           "JSON",
		"Version":          "2015-01-09",
		"AccessKeyId":      a.keyID,
		"SignatureMethod":  "HMAC-SHA1",
		"SignatureVersion": "1.0",
		"SignatureNonce":   aliyunNonce(),
		"Timestamp":        time.Now().UTC().Format("2006-01-02T15:04:05Z"),
		"Action":           action,
	}
	for k, v := range params {
		if v != "" {
			p[k] = v
		}
	}
	sig := aliyunSign(p, a.secret)

	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(a.base)
	sb.WriteByte('?')
	for _, k := range keys {
		sb.WriteString(percentEncode(k))
		sb.WriteByte('=')
		sb.WriteString(percentEncode(p[k]))
		sb.WriteByte('&')
	}
	sb.WriteString("Signature=")
	sb.WriteString(percentEncode(sig))

	req, err := http.NewRequestWithContext(ctx, "GET", sb.String(), nil)
	if err != nil {
		return err
	}
	resp, err := a.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		var e struct {
			Code    string `json:"Code"`
			Message string `json:"Message"`
		}
		_ = json.Unmarshal(raw, &e)
		if e.Message == "" {
			e.Message = strings.TrimSpace(string(raw))
		}
		return fmt.Errorf("aliyun %s: %s %s", action, e.Code, e.Message)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("aliyun %s: 响应解析失败: %v", action, err)
	}
	return nil
}

func (a *aliyun) Verify(ctx context.Context) error {
	var r aliDomainsResp
	return a.call(ctx, "DescribeDomains", map[string]string{"PageNumber": "1", "PageSize": "1"}, &r)
}

type aliDomain struct {
	DomainName string `json:"DomainName"`
	DomainID   string `json:"DomainId"`
}

type aliDomainsResp struct {
	TotalCount int `json:"TotalCount"`
	Domains    struct {
		Domain []aliDomain `json:"Domain"`
	} `json:"Domains"`
}

func (a *aliyun) ListZones(ctx context.Context) ([]Zone, error) {
	var out []Zone
	for page := 1; page <= 100; page++ {
		var r aliDomainsResp
		err := a.call(ctx, "DescribeDomains", map[string]string{
			"PageNumber": strconv.Itoa(page),
			"PageSize":   "100",
		}, &r)
		if err != nil {
			return nil, err
		}
		for _, d := range r.Domains.Domain {
			out = append(out, Zone{ID: d.DomainID, Name: d.DomainName})
		}
		if len(r.Domains.Domain) == 0 || page*100 >= r.TotalCount {
			break
		}
	}
	return out, nil
}

type aliRecord struct {
	RecordID string `json:"RecordId"`
	RR       string `json:"RR"`
	Type     string `json:"Type"`
	Value    string `json:"Value"`
	TTL      int    `json:"TTL"`
	Priority int    `json:"Priority"`
	Line     string `json:"Line"`
	Status   string `json:"Status"`
}

type aliRecordsResp struct {
	TotalCount    int `json:"TotalCount"`
	DomainRecords struct {
		Record []aliRecord `json:"Record"`
	} `json:"DomainRecords"`
}

func (a *aliyun) findRecord(ctx context.Context, zone string, r Record) (*aliRecord, error) {
	params := map[string]string{
		"DomainName":  zone,
		"TypeKeyWord": strings.ToUpper(r.Type),
		"PageNumber":  "1",
		"PageSize":    "100",
	}
	// RRKeyWord 为模糊匹配，根记录（@）不传，避免匹配不到而重复添加
	if r.Name != "@" && r.Name != "" {
		params["RRKeyWord"] = r.Name
	}
	var resp aliRecordsResp
	if err := a.call(ctx, "DescribeDomainRecords", params, &resp); err != nil {
		return nil, err
	}
	for i := range resp.DomainRecords.Record {
		rec := resp.DomainRecords.Record[i]
		if strings.EqualFold(rec.RR, r.Name) && strings.EqualFold(rec.Type, r.Type) {
			return &rec, nil
		}
	}
	return nil, nil
}

func (a *aliyun) EnsureRecords(ctx context.Context, zone string, records []Record) ([]Result, error) {
	zone = normHost(zone)
	out := make([]Result, 0, len(records))
	for _, r := range records {
		res := Result{Name: r.Name, Type: r.Type, Value: r.Value}
		existing, err := a.findRecord(ctx, zone, r)
		if err != nil {
			res.Action, res.Error = "failed", err.Error()
			out = append(out, res)
			continue
		}
		base := map[string]string{
			"RR":    r.Name,
			"Type":  strings.ToUpper(r.Type),
			"Value": r.Value,
			"TTL":   strconv.Itoa(aliTTL(r.TTL)),
			"Line":  "default",
		}
		if isPriorityType(r.Type) {
			base["Priority"] = strconv.Itoa(r.Priority)
		}
		if existing != nil {
			if existing.Value == r.Value && (!isPriorityType(r.Type) || existing.Priority == r.Priority) {
				res.Action = "unchanged"
				out = append(out, res)
				continue
			}
			base["RecordId"] = existing.RecordID
			if err := a.call(ctx, "UpdateDomainRecord", base, nil); err != nil {
				res.Action, res.Error = "failed", err.Error()
			} else {
				res.Action = "updated"
			}
		} else {
			base["DomainName"] = zone
			if err := a.call(ctx, "AddDomainRecord", base, nil); err != nil {
				res.Action, res.Error = "failed", err.Error()
			} else {
				res.Action = "created"
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// aliTTL 归一到阿里云常用取值（600~86400）。
func aliTTL(ttl int) int {
	for _, a := range []int{600, 1800, 3600, 43200, 86400} {
		if ttl <= a {
			return a
		}
	}
	return 600
}
