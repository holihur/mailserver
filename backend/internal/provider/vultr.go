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

// ---- Vultr DNS ----
// 凭证：api_token（Personal Access Token）。
// 文档：https://www.vultr.com/api/#tag/DNS

type vultr struct {
	token string
	http  *http.Client
	base  string
}

func newVultr(creds map[string]string) (*vultr, error) {
	v := &vultr{
		token: strings.TrimSpace(creds["api_token"]),
		http:  &http.Client{Timeout: 20 * time.Second},
		base:  "https://api.vultr.com",
	}
	if v.token == "" {
		return nil, fmt.Errorf("vultr: 需要 API Token")
	}
	return v, nil
}

type vultrEnvelope struct {
	Domains []vultrDomain `json:"domains"`
	Records []vultrRecord `json:"records"`
	Meta    struct {
		Total int `json:"total"`
		Links struct {
			Next string `json:"next"`
		} `json:"links"`
	} `json:"meta"`
	Error string `json:"error"`
}

type vultrDomain struct {
	Domain string `json:"domain"`
}

type vultrRecord struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Data     string `json:"data"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority"`
}

func (v *vultr) do(ctx context.Context, method, path string, body any) (*vultrEnvelope, error) {
	target := path
	if !strings.HasPrefix(path, "http") {
		target = v.base + path
	}
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rdr)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+v.token)
	resp, err := v.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var env vultrEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("vultr: 响应解析失败 (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if resp.StatusCode >= 400 {
		msg := env.Error
		if msg == "" {
			msg = strings.TrimSpace(string(raw))
		}
		return &env, fmt.Errorf("vultr: %s (HTTP %d)", msg, resp.StatusCode)
	}
	return &env, nil
}

func (v *vultr) Verify(ctx context.Context) error {
	_, err := v.do(ctx, "GET", "/v2/domains?per_page=1", nil)
	return err
}

func (v *vultr) ListZones(ctx context.Context) ([]Zone, error) {
	var out []Zone
	path := "/v2/domains?per_page=500"
	for i := 0; i < 100 && path != ""; i++ {
		env, err := v.do(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}
		for _, z := range env.Domains {
			out = append(out, Zone{ID: z.Domain, Name: z.Domain})
		}
		path = env.Meta.Links.Next
	}
	return out, nil
}

func (v *vultr) findRecord(ctx context.Context, zone, name, typ string) (*vultrRecord, error) {
	path := "/v2/domains/" + url.PathEscape(zone) + "/records?per_page=500"
	for i := 0; i < 100 && path != ""; i++ {
		env, err := v.do(ctx, "GET", path, nil)
		if err != nil {
			return nil, err
		}
		for j := range env.Records {
			r := env.Records[j]
			if sameName(r.Name, name) && strings.EqualFold(r.Type, typ) {
				return &r, nil
			}
		}
		path = env.Meta.Links.Next
	}
	return nil, nil
}

func (v *vultr) EnsureRecords(ctx context.Context, zone string, records []Record) ([]Result, error) {
	zone = normHost(zone)
	out := make([]Result, 0, len(records))
	for _, r := range records {
		res := Result{Name: r.Name, Type: r.Type, Value: r.Value}
		existing, err := v.findRecord(ctx, zone, r.Name, strings.ToUpper(r.Type))
		if err != nil {
			res.Action, res.Error = "failed", err.Error()
			out = append(out, res)
			continue
		}
		payload := map[string]any{
			"type": strings.ToUpper(r.Type),
			"name": emptyRoot(r.Name),
			"data": r.Value,
			"ttl":  vultrTTL(r.TTL),
		}
		if isPriorityType(r.Type) {
			payload["priority"] = r.Priority
		}
		if existing != nil {
			if existing.Data == r.Value && (!isPriorityType(r.Type) || existing.Priority == r.Priority) {
				res.Action = "unchanged"
				out = append(out, res)
				continue
			}
			p := "/v2/domains/" + url.PathEscape(zone) + "/records/" + url.PathEscape(existing.ID)
			if _, err := v.do(ctx, "PATCH", p, payload); err != nil {
				res.Action, res.Error = "failed", err.Error()
			} else {
				res.Action = "updated"
			}
		} else {
			if _, err := v.do(ctx, "POST", "/v2/domains/"+url.PathEscape(zone)+"/records", payload); err != nil {
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
func (v *vultr) DeleteRecord(ctx context.Context, zone, name, typ, value string) error {
	rec, err := v.findRecord(ctx, normHost(zone), name, typ)
	if err != nil {
		return err
	}
	if rec == nil || (value != "" && rec.Data != value) {
		return nil
	}
	_, err = v.do(ctx, "DELETE", "/v2/domains/"+url.PathEscape(normHost(zone))+"/records/"+url.PathEscape(rec.ID), nil)
	return err
}

// vultrTTL 把 TTL 归一到 Vultr 允许的取值（300~86400）。
func vultrTTL(ttl int) int {
	if ttl <= 300 {
		return 300
	}
	for _, a := range []int{300, 600, 900, 1800, 3600, 7200, 14400, 28800, 43200, 86400} {
		if ttl <= a {
			return a
		}
	}
	return 86400
}
