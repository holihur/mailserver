package emailauth

import (
	"strings"
)

// ---- DMARC ----

// DMARC 查询发件域的 DMARC 策略（none/quarantine/reject）。present=false 表示无记录。
func DMARC(l Lookup, fromDomain string) (policy string, present bool) {
	fromDomain = strings.ToLower(strings.TrimSpace(fromDomain))
	if fromDomain == "" {
		return "", false
	}
	for _, d := range []string{fromDomain, OrgDomain(fromDomain)} {
		txts, err := l.TXT("_dmarc." + d)
		if err != nil {
			continue
		}
		for _, t := range txts {
			t = strings.TrimSpace(t)
			if strings.HasPrefix(strings.ToLower(t), "v=dmarc1") {
				return dmarcPolicy(t), true
			}
		}
	}
	return "", false
}

func dmarcPolicy(rec string) string {
	for _, p := range strings.Split(rec, ";") {
		p = strings.TrimSpace(p)
		if i := strings.IndexByte(p, '='); i > 0 && strings.EqualFold(strings.TrimSpace(p[:i]), "p") {
			v := strings.ToLower(strings.TrimSpace(p[i+1:]))
			switch v {
			case "none", "quarantine", "reject":
				return v
			}
		}
	}
	return "none"
}

// OrgDomain 粗略取组织域（最后两级标签）。
func OrgDomain(host string) string {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), ".")
	parts := strings.Split(host, ".")
	if len(parts) <= 2 {
		return host
	}
	return strings.Join(parts[len(parts)-2:], ".")
}

// Aligned 判断 fromDomain 与认证域是否对齐（relaxed 时比较组织域）。
func Aligned(fromDomain, authDomain string, relaxed bool) bool {
	fromDomain = strings.ToLower(strings.TrimSpace(fromDomain))
	authDomain = strings.ToLower(strings.TrimSpace(authDomain))
	if fromDomain == "" || authDomain == "" {
		return false
	}
	if fromDomain == authDomain {
		return true
	}
	return relaxed && OrgDomain(fromDomain) == OrgDomain(authDomain)
}
