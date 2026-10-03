package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ---- Cloudflare API v4 ----
// 凭证：api_token（推荐）或 email + api_key（Global API Key）。
// 文档：https://developers.cloudflare.com/api/

type cloudflare struct {
	token string
	email string
	key   string
	http  *http.Client
	base  string
}

func newCloudflare(creds map[string]string) (*cloudflare, error) {
	c := &cloudflare{
		token: strings.TrimSpace(creds["api_token"]),
		email: strings.TrimSpace(creds["email"]),
		key:   strings.TrimSpace(creds["api_key"]),
		http:  &http.Client{Timeout: 20 * time.Second},
		base:  "https://api.cloudflare.com/client/v4",
	}
	if c.token == "" && (c.email == "" || c.key == "") {
		return nil, errors.New("cloudflare: 需要 API Token，或 Email + Global API Key")
	}
	return c, nil
}

type cfError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type cfEnvelope struct {
	Success    bool            `json:"success"`
	Errors     []cfError       `json:"errors"`
	Result     json.RawMessage `json:"result"`
	ResultInfo struct {
		Page       int `json:"page"`
		TotalPages int `json:"total_pages"`
	} `json:"result_info"`
}

func cfErrText(errs []cfError, status int) string {
	if len(errs) == 0 {
		return fmt.Sprintf("HTTP %d", status)
	}
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, fmt.Sprintf("%d %s", e.Code, e.Message))
	}
	return strings.Join(parts, "; ")
}

func (c *cloudflare) do(ctx context.Context, method, path string, body any) (*cfEnvelope, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	} else {
		req.Header.Set("X-Auth-Email", c.email)
		req.Header.Set("X-Auth-Key", c.key)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env cfEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("cloudflare: 响应解析失败 (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if !env.Success {
		return &env, fmt.Errorf("cloudflare: %s", cfErrText(env.Errors, resp.StatusCode))
	}
	return &env, nil
}

func (c *cloudflare) Verify(ctx context.Context) error {
	_, err := c.do(ctx, "GET", "/zones?per_page=1", nil)
	return err
}

type cfZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (c *cloudflare) ListZones(ctx context.Context) ([]Zone, error) {
	var out []Zone
	for page := 1; page <= 100; page++ {
		env, err := c.do(ctx, "GET", fmt.Sprintf("/zones?per_page=50&page=%d", page), nil)
		if err != nil {
			return nil, err
		}
		var zs []cfZone
		if err := json.Unmarshal(env.Result, &zs); err != nil {
			return nil, err
		}
		for _, z := range zs {
			out = append(out, Zone{ID: z.ID, Name: z.Name})
		}
		if env.ResultInfo.TotalPages == 0 || page >= env.ResultInfo.TotalPages {
			break
		}
	}
	return out, nil
}

type cfRecord struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Content  string `json:"content"`
	Priority int    `json:"priority"`
	TTL      int    `json:"ttl"`
}

func (c *cloudflare) zoneID(ctx context.Context, zone string) (string, error) {
	env, err := c.do(ctx, "GET", "/zones?name="+url.QueryEscape(zone), nil)
	if err != nil {
		return "", err
	}
	var zs []cfZone
	if err := json.Unmarshal(env.Result, &zs); err != nil {
		return "", err
	}
	if len(zs) == 0 {
		return "", fmt.Errorf("cloudflare: 账号下未找到域名 %s（检查该域名是否已添加到 Cloudflare）", zone)
	}
	return zs[0].ID, nil
}

func (c *cloudflare) findRecord(ctx context.Context, zoneID, typ, fqdn string) (*cfRecord, error) {
	p := fmt.Sprintf("/zones/%s/dns_records?type=%s&name=%s&per_page=100",
		zoneID, url.QueryEscape(strings.ToUpper(typ)), url.QueryEscape(fqdn))
	env, err := c.do(ctx, "GET", p, nil)
	if err != nil {
		return nil, err
	}
	var rs []cfRecord
	if err := json.Unmarshal(env.Result, &rs); err != nil {
		return nil, err
	}
	if len(rs) == 0 {
		return nil, nil
	}
	return &rs[0], nil
}

func (c *cloudflare) EnsureRecords(ctx context.Context, zone string, records []Record) ([]Result, error) {
	zone = normHost(zone)
	zid, err := c.zoneID(ctx, zone)
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(records))
	for _, r := range records {
		res := Result{Name: r.Name, Type: r.Type, Value: r.Value}
		fqdn := recordFQDN(zone, r.Name)
		existing, err := c.findRecord(ctx, zid, r.Type, fqdn)
		if err != nil {
			res.Action, res.Error = "failed", err.Error()
			out = append(out, res)
			continue
		}
		payload := map[string]any{
			"type":    strings.ToUpper(r.Type),
			"name":    fqdn,
			"content": r.Value,
			"ttl":     cfTTL(r.TTL),
			"proxied": false,
		}
		if isPriorityType(r.Type) {
			payload["priority"] = r.Priority
		}
		if existing != nil {
			if existing.Content == r.Value && (!isPriorityType(r.Type) || existing.Priority == r.Priority) {
				res.Action = "unchanged"
				out = append(out, res)
				continue
			}
			if _, err := c.do(ctx, "PUT", "/zones/"+zid+"/dns_records/"+existing.ID, payload); err != nil {
				res.Action, res.Error = "failed", err.Error()
			} else {
				res.Action = "updated"
			}
		} else {
			if _, err := c.do(ctx, "POST", "/zones/"+zid+"/dns_records", payload); err != nil {
				res.Action, res.Error = "failed", err.Error()
			} else {
				res.Action = "created"
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// recordFQDN 把相对名转成完整域名（Cloudflare 需要完整记录名）。
func recordFQDN(zone, name string) string {
	name = normHost(name)
	if name == "" || name == "@" {
		return zone
	}
	if name == zone || strings.HasSuffix(name, "."+zone) {
		return name
	}
	return name + "." + zone
}

func isPriorityType(t string) bool {
	t = strings.ToUpper(t)
	return t == "MX" || t == "SRV"
}

// cfTTL 把 TTL 归一到 Cloudflare 允许的取值。
func cfTTL(ttl int) int {
	if ttl <= 1 {
		return 1
	}
	for _, a := range []int{60, 120, 300, 600, 1800, 3600, 7200, 10800, 21600, 43200, 86400} {
		if ttl <= a {
			return a
		}
	}
	return 86400
}
