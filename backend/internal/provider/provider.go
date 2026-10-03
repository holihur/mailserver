// Package provider 封装第三方域名服务商（阿里云 DNS / Cloudflare）的解析记录下发。
// 目标是「开箱即用」：用户录入凭证 → 列出账号下域名 → 选择域名 → 自动配齐邮件所需解析。
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Record 与具体服务商无关的一条解析记录。
// Name 为相对名：@（根）、mail、_dmarc、<selector>._domainkey。
type Record struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Value    string `json:"value"`
	TTL      int    `json:"ttl"`
	Priority int    `json:"priority,omitempty"`
}

// Zone 服务商账号下的一个可管理域名。
type Zone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Result 单条记录的下发结果。
type Result struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	Action string `json:"action"` // created | updated | unchanged | failed
	Error  string `json:"error,omitempty"`
}

// Provider 是各服务商客户端的统一接口。
type Provider interface {
	// Verify 校验凭证是否可用。
	Verify(ctx context.Context) error
	// ListZones 列出账号下可管理的域名。
	ListZones(ctx context.Context) ([]Zone, error)
	// EnsureRecords 在 zone 下创建/更新记录（同主机名+类型已存在则更新），返回逐条结果。
	EnsureRecords(ctx context.Context, zone string, records []Record) ([]Result, error)
	// DeleteRecord 删除匹配（name+type，value 非空时也需相等）的记录，不存在不报错。
	DeleteRecord(ctx context.Context, zone, name, typ, value string) error
}

// New 根据类型与凭证构造客户端。creds 的键见各自的构造函数。
func New(kind string, creds map[string]string) (Provider, error) {
	switch kind {
	case "aliyun":
		return newAliyun(creds)
	case "cloudflare":
		return newCloudflare(creds)
	default:
		return nil, fmt.Errorf("不支持的域名服务商: %q", kind)
	}
}

// DecodeCreds 解析凭证 JSON。
func DecodeCreds(raw []byte) (map[string]string, error) {
	m := map[string]string{}
	if len(raw) == 0 {
		return m, nil
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("解析凭证失败: %w", err)
	}
	return m, nil
}

// normHost 去掉末尾的点，统一小写。
func normHost(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		out = append(out, c)
	}
	for len(out) > 0 && out[len(out)-1] == '.' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// MatchZone 从账号域名中找出最能匹配 target 的 zone（后缀最长者）。
func MatchZone(zones []Zone, target string) (Zone, bool) {
	target = normHost(target)
	var best Zone
	found := false
	for _, z := range zones {
		name := normHost(z.Name)
		if name == "" {
			continue
		}
		if target == name || strings.HasSuffix(target, "."+name) {
			if !found || len(name) > len(normHost(best.Name)) {
				best, found = z, true
			}
		}
	}
	return best, found
}

// RelativeName 把完整记录名转成相对 zone 的名字（zone 本身为 @）。
func RelativeName(zone, fqdn string) string {
	zone = normHost(zone)
	fqdn = normHost(fqdn)
	if fqdn == zone {
		return "@"
	}
	if strings.HasSuffix(fqdn, "."+zone) {
		return strings.TrimSuffix(fqdn, "."+zone)
	}
	return fqdn
}
