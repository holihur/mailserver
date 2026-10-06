package handler

// 管理后台：系统设置 + TLS/SSL（手动上传 / Let's Encrypt 自动签发）+ DKIM 密钥管理。
// 目标是「所有配置都在后台点」，命令行只保留数据库/JWT 等必要引导项。

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"mailserver/internal/runtimecfg"
)

var settingKeys = map[string]bool{
	runtimecfg.KeyMailHost:           true,
	runtimecfg.KeyPublicIP:           true,
	runtimecfg.KeyAdminEmails:        true,
	runtimecfg.KeyRelayHost:          true,
	runtimecfg.KeyRelayPort:          true,
	runtimecfg.KeyRelayUser:          true,
	runtimecfg.KeyRelayPass:          true,
	runtimecfg.KeyRelayFrom:          true,
	runtimecfg.KeyRelayInsecure:      true,
	runtimecfg.KeyDirectSend:         true,
	runtimecfg.KeyRegistration:       true,
	runtimecfg.KeyAutoUpdate:         true,
	runtimecfg.KeyUpdateInterval:     true,
	runtimecfg.KeyOIDCEnabled:        true,
	runtimecfg.KeyOIDCIssuer:         true,
	runtimecfg.KeyOIDCClientID:       true,
	runtimecfg.KeyOIDCClientSecret:   true,
	runtimecfg.KeyOIDCAutoCreate:     true,
	runtimecfg.KeyBackupDir:          true,
	runtimecfg.KeyBackupInterval:     true,
	runtimecfg.KeyBackupKeep:         true,
	runtimecfg.KeyLoginRetentionDays: true,
	runtimecfg.KeyAuditRetentionDays: true,
}

// GET /api/admin/settings   PATCH /api/admin/settings
func (a *Admin) Settings(w http.ResponseWriter, r *http.Request) {
	if _, ok := a.mustAdmin(w, r); !ok {
		return
	}
	switch r.Method {
	case "GET":
		writeJSON(w, 200, a.RT.Snapshot())
	case "PATCH":
		var in map[string]string
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&in); err != nil {
			writeJSON(w, 400, map[string]string{"error": "bad body"})
			return
		}
		upd := map[string]string{}
		for k, v := range in {
			if !settingKeys[k] {
				continue
			}
			v = strings.TrimSpace(v)
			// 密码留空表示不修改
			if k == runtimecfg.KeyRelayPass && v == "" {
				continue
			}
			if k == runtimecfg.KeyOIDCClientSecret && v == "" {
				continue
			}
			if k == runtimecfg.KeyMailHost {
				v = strings.Trim(strings.ToLower(v), ".")
			}
			if k == runtimecfg.KeyRegistration || k == runtimecfg.KeyRelayInsecure || k == runtimecfg.KeyDirectSend || k == runtimecfg.KeyAutoUpdate || k == runtimecfg.KeyOIDCEnabled || k == runtimecfg.KeyOIDCAutoCreate {
				if v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "on") {
					v = "1"
				} else {
					v = "0"
				}
			}
			if k == runtimecfg.KeyUpdateInterval {
				n, err := strconv.Atoi(v)
				if err != nil || n <= 0 {
					n = 10
				}
				if n > 1440 {
					n = 1440
				}
				v = strconv.Itoa(n)
			}
			if k == runtimecfg.KeyBackupInterval {
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 {
					n = 24
				}
				if n > 8760 {
					n = 8760
				}
				v = strconv.Itoa(n)
			}
			if k == runtimecfg.KeyBackupKeep {
				n, err := strconv.Atoi(v)
				if err != nil || n < 1 {
					n = 7
				}
				if n > 3650 {
					n = 3650
				}
				v = strconv.Itoa(n)
			}
			if k == runtimecfg.KeyLoginRetentionDays {
				v = clampDays(v, 90)
			}
			if k == runtimecfg.KeyAuditRetentionDays {
				v = clampDays(v, 180)
			}
			upd[k] = v
		}
		if h, ok := upd[runtimecfg.KeyMailHost]; ok && h != "" && !strings.Contains(h, ".") {
			writeJSON(w, 400, map[string]string{"error": "邮件域名格式不正确，例如 mail.example.com"})
			return
		}
		a.RT.SetMany(upd)
		writeJSON(w, 200, a.RT.Snapshot())
	default:
		w.WriteHeader(405)
	}
}

// GET /api/admin/tls  -> 证书状态 + ACME 配置 + 可选服务商
// DELETE /api/admin/tls -> 删除证书

func normalizeDomain(s string) string {
	return strings.Trim(strings.ToLower(strings.TrimSpace(s)), ".")
}

// deriveBaseDomain mail.example.com -> example.com；example.com -> example.com
func deriveBaseDomain(host string) string {
	h := strings.Trim(strings.ToLower(strings.TrimSpace(host)), ".")
	if h == "" {
		return ""
	}
	parts := strings.Split(h, ".")
	if len(parts) > 2 {
		return strings.Join(parts[len(parts)-2:], ".")
	}
	return h
}

// clampDays 解析并限制保留天数（1~3650），非法时用 def。
func clampDays(v string, def int) string {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		n = def
	}
	if n > 3650 {
		n = 3650
	}
	return strconv.Itoa(n)
}
