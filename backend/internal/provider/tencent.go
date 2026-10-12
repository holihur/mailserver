package provider

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ---- 腾讯云 DNSPod / 云解析 DNS（Tencent Cloud API v3）----
// 凭证：secret_id + secret_key（推荐用 CAM 子账号，授予 QcloudDNSPodFullAccess 策略）。
// 文档：https://cloud.tencent.com/document/api/1427/56164
//
// 腾讯云自 2021 起将 DNSPod 作为「云解析 DNS」的内核，API 域名为
// dnspod.tencentcloudapi.com，服务名为 dnspod，接口版本 2021-03-23。

const tencentAPIVersion = "2021-03-23"

type tencent struct {
	secretID  string
	secretKey string
	http      *http.Client
	base      string
}

func newTencent(creds map[string]string) (*tencent, error) {
	t := &tencent{
		secretID:  strings.TrimSpace(creds["secret_id"]),
		secretKey: strings.TrimSpace(creds["secret_key"]),
		http:      &http.Client{Timeout: 20 * time.Second},
		base:      "https://dnspod.tencentcloudapi.com",
	}
	if t.secretID == "" || t.secretKey == "" {
		return nil, fmt.Errorf("tencent: 需要 SecretId 和 SecretKey")
	}
	return t, nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key []byte, data string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(data))
	return m.Sum(nil)
}

// tencentSign 按腾讯云 TC3-HMAC-SHA256 规范计算 Authorization。
// 签名只覆盖 content-type / host 两个头（与官方示例一致）。
func tencentSign(secretID, secretKey, host string, payload []byte, ts int64) string {
	date := time.Unix(ts, 0).UTC().Format("2006-01-02")
	credentialScope := date + "/dnspod/tc3_request"

	canonicalHeaders := "content-type:application/json; charset=utf-8\nhost:" + host + "\n"
	signedHeaders := "content-type;host"
	canonicalRequest := "POST\n/\n\n" + canonicalHeaders + "\n" + signedHeaders + "\n" + sha256Hex(payload)
	hashedCanonical := sha256Hex([]byte(canonicalRequest))

	stringToSign := "TC3-HMAC-SHA256\n" + strconv.FormatInt(ts, 10) + "\n" + credentialScope + "\n" + hashedCanonical

	secretDate := hmacSHA256([]byte("TC3"+secretKey), date)
	secretService := hmacSHA256(secretDate, "dnspod")
	secretSigning := hmacSHA256(secretService, "tc3_request")
	signature := hex.EncodeToString(hmacSHA256(secretSigning, stringToSign))

	return "TC3-HMAC-SHA256 Credential=" + secretID + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders + ", Signature=" + signature
}

type tencentAPIError struct {
	Code    string `json:"Code"`
	Message string `json:"Message"`
}

func (t *tencent) call(ctx context.Context, action string, params any, out any) error {
	if params == nil {
		params = map[string]any{}
	}
	payload, err := json.Marshal(params)
	if err != nil {
		return err
	}
	u, err := url.Parse(t.base)
	if err != nil {
		return err
	}
	host := u.Host
	ts := time.Now().Unix()

	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(t.base, "/")+"/", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Host = host
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-TC-Action", action)
	req.Header.Set("X-TC-Timestamp", strconv.FormatInt(ts, 10))
	req.Header.Set("X-TC-Version", tencentAPIVersion)
	req.Header.Set("Authorization", tencentSign(t.secretID, t.secretKey, host, payload, ts))

	resp, err := t.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	var env struct {
		Response json.RawMessage `json:"Response"`
	}
	if err := json.Unmarshal(raw, &env); err != nil || len(env.Response) == 0 {
		return fmt.Errorf("tencent %s: 响应解析失败 (HTTP %d): %s", action, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var e struct {
		Error *tencentAPIError `json:"Error"`
	}
	if err := json.Unmarshal(env.Response, &e); err == nil && e.Error != nil {
		return fmt.Errorf("tencent %s: %s %s", action, e.Error.Code, e.Error.Message)
	}
	if resp.StatusCode >= 400 {
		return fmt.Errorf("tencent %s: HTTP %d: %s", action, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(env.Response, out); err != nil {
		return fmt.Errorf("tencent %s: 响应解析失败: %v", action, err)
	}
	return nil
}

func (t *tencent) Verify(ctx context.Context) error {
	var r tencentDomainListResp
	return t.call(ctx, "DescribeDomainList", map[string]any{"Offset": 0, "Limit": 1}, &r)
}

type tencentDomain struct {
	DomainID uint64 `json:"DomainId"`
	Name     string `json:"Name"`
}

type tencentDomainListResp struct {
	DomainList []tencentDomain `json:"DomainList"`
}

func (t *tencent) ListZones(ctx context.Context) ([]Zone, error) {
	const pageSize = 100
	var out []Zone
	for offset := 0; offset < 100000; offset += pageSize {
		var r tencentDomainListResp
		err := t.call(ctx, "DescribeDomainList", map[string]any{
			"Offset": offset,
			"Limit":  pageSize,
		}, &r)
		if err != nil {
			return nil, err
		}
		for _, d := range r.DomainList {
			out = append(out, Zone{ID: strconv.FormatUint(d.DomainID, 10), Name: d.Name})
		}
		if len(r.DomainList) < pageSize {
			break
		}
	}
	return out, nil
}

type tencentRecord struct {
	RecordID uint64 `json:"RecordId"`
	Name     string `json:"Name"`
	Type     string `json:"Type"`
	Value    string `json:"Value"`
	TTL      uint64 `json:"TTL"`
	MX       uint64 `json:"MX"`
	Line     string `json:"Line"`
	Status   string `json:"Status"`
}

type tencentRecordListResp struct {
	RecordList []tencentRecord `json:"RecordList"`
}

// findRecord 按子域名 + 类型精确查找记录（DNSPod 的 Subdomain 为过滤条件，这里再做一次精确比对）。
func (t *tencent) findRecord(ctx context.Context, zone string, r Record) (*tencentRecord, error) {
	const pageSize = 100
	for offset := 0; offset < 100000; offset += pageSize {
		params := map[string]any{
			"Domain": zone,
			"Limit":  pageSize,
			"Offset": offset,
		}
		if r.Name != "" {
			params["Subdomain"] = r.Name
		}
		if r.Type != "" {
			params["RecordType"] = strings.ToUpper(r.Type)
		}
		var resp tencentRecordListResp
		if err := t.call(ctx, "DescribeRecordList", params, &resp); err != nil {
			return nil, err
		}
		for i := range resp.RecordList {
			rec := resp.RecordList[i]
			if strings.EqualFold(rec.Name, r.Name) && strings.EqualFold(rec.Type, r.Type) {
				return &rec, nil
			}
		}
		if len(resp.RecordList) < pageSize {
			break
		}
	}
	return nil, nil
}

func (t *tencent) EnsureRecords(ctx context.Context, zone string, records []Record) ([]Result, error) {
	zone = normHost(zone)
	out := make([]Result, 0, len(records))
	for _, r := range records {
		res := Result{Name: r.Name, Type: r.Type, Value: r.Value}
		existing, err := t.findRecord(ctx, zone, r)
		if err != nil {
			res.Action, res.Error = "failed", err.Error()
			out = append(out, res)
			continue
		}
		base := map[string]any{
			"Domain":     zone,
			"SubDomain":  r.Name,
			"RecordType": strings.ToUpper(r.Type),
			"RecordLine": "默认",
			"Value":      r.Value,
			"TTL":        tencentTTL(r.TTL),
		}
		if isPriorityType(r.Type) {
			base["MX"] = r.Priority
		}
		if existing != nil {
			if existing.Value == r.Value && (!isPriorityType(r.Type) || int(existing.MX) == r.Priority) {
				res.Action = "unchanged"
				out = append(out, res)
				continue
			}
			base["RecordId"] = existing.RecordID
			if err := t.call(ctx, "ModifyRecord", base, nil); err != nil {
				res.Action, res.Error = "failed", err.Error()
			} else {
				res.Action = "updated"
			}
		} else {
			if err := t.call(ctx, "CreateRecord", base, nil); err != nil {
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
func (t *tencent) DeleteRecord(ctx context.Context, zone, name, typ, value string) error {
	zone = normHost(zone)
	rec, err := t.findRecord(ctx, zone, Record{Name: name, Type: typ})
	if err != nil {
		return err
	}
	if rec == nil {
		return nil
	}
	if value != "" && rec.Value != value {
		return nil
	}
	return t.call(ctx, "DeleteRecord", map[string]any{"Domain": zone, "RecordId": rec.RecordID}, nil)
}

// tencentTTL 把 TTL 归一到 DNSPod 允许的常用取值。
func tencentTTL(ttl int) int {
	if ttl <= 1 {
		return 1
	}
	for _, a := range []int{60, 120, 180, 300, 600, 900, 1800, 3600, 7200, 14400, 21600, 43200, 86400} {
		if ttl <= a {
			return a
		}
	}
	return 86400
}
