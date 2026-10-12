package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ---- GoDaddy DNS ----
// 凭证：api_key（Key）+ api_secret（Secret），请求头 Authorization: sso-key key:secret。
// 文档：https://developer.godaddy.com/doc/endpoint/domains

type godaddy struct {
	key    string
	secret string
	http   *http.Client
	base   string
}

func newGoDaddy(creds map[string]string) (*godaddy, error) {
	g := &godaddy{
		key:    strings.TrimSpace(creds["api_key"]),
		secret: strings.TrimSpace(creds["api_secret"]),
		http:   &http.Client{Timeout: 20 * time.Second},
		base:   "https://api.godaddy.com",
	}
	if g.key == "" || g.secret == "" {
		return nil, fmt.Errorf("godaddy: 需要 API Key 和 API Secret")
	}
	return g, nil
}

type godaddyDomain struct {
	DomainID string `json:"domainId"`
	Domain   string `json:"domain"`
	Status   string `json:"status"`
}

type godaddyRecord struct {
	Type     string `json:"type,omitempty"`
	Name     string `json:"name,omitempty"`
	Data     string `json:"data"`
	TTL      int    `json:"ttl,omitempty"`
	Priority int    `json:"priority,omitempty"`
}

type godaddyError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (g *godaddy) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, g.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "sso-key "+g.key+":"+g.secret)
	resp, err := g.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		var e godaddyError
		_ = json.Unmarshal(raw, &e)
		msg := e.Message
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		if e.Code != "" {
			msg = e.Code + " " + msg
		}
		return fmt.Errorf("godaddy: %s (HTTP %d)", msg, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("godaddy: 响应解析失败: %v", err)
	}
	return nil
}

func (g *godaddy) Verify(ctx context.Context) error {
	var ds []godaddyDomain
	return g.do(ctx, "GET", "/v1/domains?limit=1", nil, &ds)
}

func (g *godaddy) ListZones(ctx context.Context) ([]Zone, error) {
	const pageSize = 100
	var out []Zone
	for offset := 0; offset < 100000; offset += pageSize {
		var ds []godaddyDomain
		p := fmt.Sprintf("/v1/domains?limit=%d&offset=%d", pageSize, offset)
		if err := g.do(ctx, "GET", p, nil, &ds); err != nil {
			return nil, err
		}
		for _, d := range ds {
			out = append(out, Zone{ID: d.DomainID, Name: d.Domain})
		}
		if len(ds) < pageSize {
			break
		}
	}
	return out, nil
}

func (g *godaddy) records(ctx context.Context, zone, typ, name string) ([]godaddyRecord, error) {
	p := fmt.Sprintf("/v1/domains/%s/records/%s/%s",
		url.PathEscape(normHost(zone)), url.PathEscape(strings.ToUpper(typ)), url.PathEscape(dnsName(name)))
	var rs []godaddyRecord
	if err := g.do(ctx, "GET", p, nil, &rs); err != nil {
		// 记录不存在时 GoDaddy 可能返回 404，视为空即可。
		if strings.Contains(err.Error(), "HTTP 404") {
			return nil, nil
		}
		return nil, err
	}
	return rs, nil
}

func (g *godaddy) putRecords(ctx context.Context, zone, typ, name string, rs []godaddyRecord) error {
	p := fmt.Sprintf("/v1/domains/%s/records/%s/%s",
		url.PathEscape(normHost(zone)), url.PathEscape(strings.ToUpper(typ)), url.PathEscape(dnsName(name)))
	return g.do(ctx, "PUT", p, rs, nil)
}

func (g *godaddy) EnsureRecords(ctx context.Context, zone string, records []Record) ([]Result, error) {
	zone = normHost(zone)
	out := make([]Result, 0, len(records))
	for _, r := range records {
		res := Result{Name: r.Name, Type: r.Type, Value: r.Value}
		rrset, err := g.records(ctx, zone, r.Type, r.Name)
		if err != nil {
			res.Action, res.Error = "failed", err.Error()
			out = append(out, res)
			continue
		}
		matched := -1
		for i := range rrset {
			if isPriorityType(r.Type) {
				if rrset[i].Priority == r.Priority {
					matched = i
					break
				}
			} else if rrset[i].Data == r.Value {
				matched = i
				break
			}
		}
		if matched >= 0 {
			res.Action = "unchanged"
			out = append(out, res)
			continue
		}
		newRec := godaddyRecord{
			Type: strings.ToUpper(r.Type),
			Name: dnsName(r.Name),
			Data: r.Value,
			TTL:  godaddyTTL(r.TTL),
		}
		if isPriorityType(r.Type) {
			newRec.Priority = r.Priority
		}
		if len(rrset) > 0 {
			rrset[0] = newRec
			res.Action = "updated"
		} else {
			rrset = append(rrset, newRec)
			res.Action = "created"
		}
		if err := g.putRecords(ctx, zone, r.Type, r.Name, rrset); err != nil {
			res.Action, res.Error = "failed", err.Error()
		}
		out = append(out, res)
	}
	return out, nil
}

// DeleteRecord 删除匹配的记录（不存在也不报错）。
func (g *godaddy) DeleteRecord(ctx context.Context, zone, name, typ, value string) error {
	zone = normHost(zone)
	rrset, err := g.records(ctx, zone, typ, name)
	if err != nil {
		return err
	}
	if len(rrset) == 0 {
		return nil
	}
	if value == "" {
		p := fmt.Sprintf("/v1/domains/%s/records/%s/%s",
			url.PathEscape(zone), url.PathEscape(strings.ToUpper(typ)), url.PathEscape(dnsName(name)))
		return g.do(ctx, "DELETE", p, nil, nil)
	}
	kept := make([]godaddyRecord, 0, len(rrset))
	for _, r := range rrset {
		if r.Data == value {
			continue
		}
		kept = append(kept, r)
	}
	if len(kept) == len(rrset) {
		return nil
	}
	if len(kept) == 0 {
		p := fmt.Sprintf("/v1/domains/%s/records/%s/%s",
			url.PathEscape(zone), url.PathEscape(strings.ToUpper(typ)), url.PathEscape(dnsName(name)))
		return g.do(ctx, "DELETE", p, nil, nil)
	}
	return g.putRecords(ctx, zone, typ, name, kept)
}

// godaddyTTL 把 TTL 归一到 GoDaddy 允许的最小值 600。
func godaddyTTL(ttl int) int {
	if ttl <= 600 {
		return 600
	}
	for _, a := range []int{600, 1800, 3600, 7200, 14400, 28800, 43200, 86400} {
		if ttl <= a {
			return a
		}
	}
	return 86400
}
