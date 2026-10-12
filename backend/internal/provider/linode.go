package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---- Linode DNS ----
// 凭证：api_token（Personal Access Token，需 Domains 读写权限）。
// 文档：https://www.linode.com/docs/api/domains/

type linode struct {
	token string
	http  *http.Client
	base  string
}

func newLinode(creds map[string]string) (*linode, error) {
	l := &linode{
		token: strings.TrimSpace(creds["api_token"]),
		http:  &http.Client{Timeout: 20 * time.Second},
		base:  "https://api.linode.com/v4",
	}
	if l.token == "" {
		return nil, fmt.Errorf("linode: 需要 API Token")
	}
	return l, nil
}

type linodeEnvelope struct {
	Data    json.RawMessage `json:"data"`
	Page    int             `json:"page"`
	Pages   int             `json:"pages"`
	Results int             `json:"results"`
	Errors  []struct {
		Reason string `json:"reason"`
	} `json:"errors"`
}

type linodeDomain struct {
	ID     int64  `json:"id"`
	Domain string `json:"domain"`
}

type linodeRecord struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	Target   string `json:"target"`
	TTLSec   int    `json:"ttl_sec"`
	Priority int    `json:"priority"`
}

func (l *linode) do(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, l.base+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+l.token)
	resp, err := l.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		var env linodeEnvelope
		_ = json.Unmarshal(raw, &env)
		msg := strings.TrimSpace(string(raw))
		if len(env.Errors) > 0 {
			parts := make([]string, 0, len(env.Errors))
			for _, e := range env.Errors {
				parts = append(parts, e.Reason)
			}
			msg = strings.Join(parts, "; ")
		}
		return fmt.Errorf("linode: %s (HTTP %d)", msg, resp.StatusCode)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("linode: 响应解析失败: %v", err)
	}
	return nil
}

func (l *linode) Verify(ctx context.Context) error {
	var env linodeEnvelope
	return l.do(ctx, "GET", "/domains?page_size=1", nil, &env)
}

func (l *linode) ListZones(ctx context.Context) ([]Zone, error) {
	var out []Zone
	for page := 1; page <= 100; page++ {
		var env struct {
			Data  []linodeDomain `json:"data"`
			Pages int            `json:"pages"`
		}
		if err := l.do(ctx, "GET", fmt.Sprintf("/domains?page=%d&page_size=100", page), nil, &env); err != nil {
			return nil, err
		}
		for _, d := range env.Data {
			out = append(out, Zone{ID: strconv.FormatInt(d.ID, 10), Name: d.Domain})
		}
		if env.Pages == 0 || page >= env.Pages {
			break
		}
	}
	return out, nil
}

// zoneID 解析域名对应的 Linode 资源 ID。
func (l *linode) zoneID(ctx context.Context, zone string) (int64, error) {
	zone = normHost(zone)
	for page := 1; page <= 100; page++ {
		var env struct {
			Data  []linodeDomain `json:"data"`
			Pages int            `json:"pages"`
		}
		if err := l.do(ctx, "GET", fmt.Sprintf("/domains?page=%d&page_size=100", page), nil, &env); err != nil {
			return 0, err
		}
		for _, d := range env.Data {
			if strings.EqualFold(normHost(d.Domain), zone) {
				return d.ID, nil
			}
		}
		if env.Pages == 0 || page >= env.Pages {
			break
		}
	}
	return 0, fmt.Errorf("linode: 账号下未找到域名 %s", zone)
}

func (l *linode) findRecord(ctx context.Context, zid int64, name, typ string) (*linodeRecord, error) {
	for page := 1; page <= 100; page++ {
		var env struct {
			Data  []linodeRecord `json:"data"`
			Pages int            `json:"pages"`
		}
		p := fmt.Sprintf("/domains/%d/records?page=%d&page_size=500", zid, page)
		if err := l.do(ctx, "GET", p, nil, &env); err != nil {
			return nil, err
		}
		for i := range env.Data {
			r := env.Data[i]
			if sameName(r.Name, name) && strings.EqualFold(r.Type, typ) {
				return &r, nil
			}
		}
		if env.Pages == 0 || page >= env.Pages {
			break
		}
	}
	return nil, nil
}

func (l *linode) EnsureRecords(ctx context.Context, zone string, records []Record) ([]Result, error) {
	zone = normHost(zone)
	zid, err := l.zoneID(ctx, zone)
	if err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(records))
	for _, r := range records {
		res := Result{Name: r.Name, Type: r.Type, Value: r.Value}
		existing, err := l.findRecord(ctx, zid, r.Name, strings.ToUpper(r.Type))
		if err != nil {
			res.Action, res.Error = "failed", err.Error()
			out = append(out, res)
			continue
		}
		payload := map[string]any{
			"type":    strings.ToUpper(r.Type),
			"name":    emptyRoot(r.Name),
			"target":  r.Value,
			"ttl_sec": linodeTTL(r.TTL),
		}
		if isPriorityType(r.Type) {
			payload["priority"] = r.Priority
		}
		if existing != nil {
			if existing.Target == r.Value && (!isPriorityType(r.Type) || existing.Priority == r.Priority) {
				res.Action = "unchanged"
				out = append(out, res)
				continue
			}
			p := fmt.Sprintf("/domains/%d/records/%d", zid, existing.ID)
			if err := l.do(ctx, "PUT", p, payload, nil); err != nil {
				res.Action, res.Error = "failed", err.Error()
			} else {
				res.Action = "updated"
			}
		} else {
			if err := l.do(ctx, "POST", fmt.Sprintf("/domains/%d/records", zid), payload, nil); err != nil {
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
func (l *linode) DeleteRecord(ctx context.Context, zone, name, typ, value string) error {
	zid, err := l.zoneID(ctx, normHost(zone))
	if err != nil {
		return err
	}
	rec, err := l.findRecord(ctx, zid, name, typ)
	if err != nil {
		return err
	}
	if rec == nil || (value != "" && rec.Target != value) {
		return nil
	}
	return l.do(ctx, "DELETE", fmt.Sprintf("/domains/%d/records/%d", zid, rec.ID), nil, nil)
}

// linodeTTL 把 TTL 归一到 Linode 允许的取值（300~86400）。
func linodeTTL(ttl int) int {
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
