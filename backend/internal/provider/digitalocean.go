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

// ---- DigitalOcean DNS ----
// 凭证：api_token（Personal Access Token，需 read/write 权限）。
// 文档：https://docs.digitalocean.com/reference/api/digitalocean/#tag/Domains

type digitalocean struct {
	token string
	http  *http.Client
	base  string
}

func newDigitalOcean(creds map[string]string) (*digitalocean, error) {
	d := &digitalocean{
		token: strings.TrimSpace(creds["api_token"]),
		http:  &http.Client{Timeout: 20 * time.Second},
		base:  "https://api.digitalocean.com/v2",
	}
	if d.token == "" {
		return nil, fmt.Errorf("digitalocean: 需要 API Token")
	}
	return d, nil
}

type doEnvelope struct {
	Domains       []doDomain `json:"domains"`
	DomainRecords []doRecord `json:"domain_records"`
	Message       string     `json:"message"`
}

type doDomain struct {
	Name string `json:"name"`
}

type doRecord struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Data     string `json:"data"`
	TTL      int    `json:"ttl"`
	Priority *int   `json:"priority"`
}

func (d *digitalocean) do(ctx context.Context, method, path string, body any) (*doEnvelope, error) {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, d.base+path, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+d.token)
	resp, err := d.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env doEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("digitalocean: 响应解析失败 (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if resp.StatusCode >= 400 {
		if env.Message == "" {
			env.Message = strings.TrimSpace(string(raw))
		}
		return &env, fmt.Errorf("digitalocean: %s (HTTP %d)", env.Message, resp.StatusCode)
	}
	return &env, nil
}

func (d *digitalocean) Verify(ctx context.Context) error {
	_, err := d.do(ctx, "GET", "/domains?per_page=1", nil)
	return err
}

func (d *digitalocean) ListZones(ctx context.Context) ([]Zone, error) {
	const pageSize = 200
	var out []Zone
	for page := 1; page <= 100; page++ {
		env, err := d.do(ctx, "GET", fmt.Sprintf("/domains?per_page=%d&page=%d", pageSize, page), nil)
		if err != nil {
			return nil, err
		}
		for _, z := range env.Domains {
			out = append(out, Zone{ID: z.Name, Name: z.Name})
		}
		if len(env.Domains) < pageSize {
			break
		}
	}
	return out, nil
}

func (d *digitalocean) findRecord(ctx context.Context, zone, name, typ string) (*doRecord, error) {
	const pageSize = 200
	for page := 1; page <= 100; page++ {
		p := fmt.Sprintf("/domains/%s/records?per_page=%d&page=%d", url.PathEscape(zone), pageSize, page)
		env, err := d.do(ctx, "GET", p, nil)
		if err != nil {
			return nil, err
		}
		for i := range env.DomainRecords {
			r := env.DomainRecords[i]
			if sameName(r.Name, name) && strings.EqualFold(r.Type, typ) {
				return &r, nil
			}
		}
		if len(env.DomainRecords) < pageSize {
			break
		}
	}
	return nil, nil
}

func (d *digitalocean) EnsureRecords(ctx context.Context, zone string, records []Record) ([]Result, error) {
	zone = normHost(zone)
	out := make([]Result, 0, len(records))
	for _, r := range records {
		res := Result{Name: r.Name, Type: r.Type, Value: r.Value}
		existing, err := d.findRecord(ctx, zone, r.Name, strings.ToUpper(r.Type))
		if err != nil {
			res.Action, res.Error = "failed", err.Error()
			out = append(out, res)
			continue
		}
		payload := map[string]any{
			"type": strings.ToUpper(r.Type),
			"name": dnsName(r.Name),
			"data": r.Value,
			"ttl":  doTTL(r.TTL),
		}
		if isPriorityType(r.Type) {
			payload["priority"] = r.Priority
		}
		if existing != nil {
			if existing.Data == r.Value && (!isPriorityType(r.Type) || intPtrOrZero(existing.Priority) == r.Priority) {
				res.Action = "unchanged"
				out = append(out, res)
				continue
			}
			p := fmt.Sprintf("/domains/%s/records/%d", url.PathEscape(zone), existing.ID)
			if _, err := d.do(ctx, "PUT", p, payload); err != nil {
				res.Action, res.Error = "failed", err.Error()
			} else {
				res.Action = "updated"
			}
		} else {
			if _, err := d.do(ctx, "POST", "/domains/"+url.PathEscape(zone)+"/records", payload); err != nil {
				res.Action, res.Error = "failed", err.Error()
			} else {
				res.Action = "created"
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// DeleteRecord 删除匹配的记录（不存在也不报错）。
func (d *digitalocean) DeleteRecord(ctx context.Context, zone, name, typ, value string) error {
	rec, err := d.findRecord(ctx, normHost(zone), name, typ)
	if err != nil {
		return err
	}
	if rec == nil || (value != "" && rec.Data != value) {
		return nil
	}
	_, err = d.do(ctx, "DELETE", fmt.Sprintf("/domains/%s/records/%d", url.PathEscape(normHost(zone)), rec.ID), nil)
	return err
}

// doTTL 把 TTL 归一到 DigitalOcean 允许的取值（30~86400）。
func doTTL(ttl int) int {
	if ttl <= 30 {
		return 30
	}
	for _, a := range []int{30, 60, 120, 300, 600, 1800, 3600, 7200, 14400, 21600, 43200, 86400} {
		if ttl <= a {
			return a
		}
	}
	return 86400
}
