package emailauth

import (
	"fmt"
	"net"
	"net/mail"
	"strings"
)

type Result struct {
	SPF    string // pass/fail/softfail/neutral/none/temperror/permerror
	DKIM   string // pass/fail/none
	DMARC  string // pass/fail/none
	Policy string // none/quarantine/reject（有 DMARC 记录时）
}

// Evaluate 计算 SPF/DKIM/DMARC 结果。envelopeFrom 为 MAIL FROM 地址。
func Evaluate(l Lookup, senderIP net.IP, envelopeFrom string, raw []byte) Result {
	envDom := domainOf(envelopeFrom)
	fromDom := FromDomain(raw)
	if fromDom == "" {
		fromDom = envDom
	}
	spf := SPF(l, senderIP, envDom)
	spfAligned := spf == SPFPass && Aligned(fromDom, envDom, true)
	dkim := VerifyDKIM(l, raw)
	dkimDom := ""
	if dkim == "pass" {
		dkimDom = DKIMDomain(raw)
	}
	dkimAligned := dkim == "pass" && Aligned(fromDom, dkimDom, true)
	policy, present := DMARC(l, fromDom)
	dmarc := "none"
	if present {
		if spfAligned || dkimAligned {
			dmarc = "pass"
		} else {
			dmarc = "fail"
		}
	} else {
		policy = ""
	}
	return Result{SPF: spf, DKIM: dkim, DMARC: dmarc, Policy: policy}
}

// FromDomain 从 RFC5322 From 头提取域（用于 DMARC）。
func FromDomain(raw []byte) string {
	headBlock, _ := splitMessage(string(raw))
	for _, h := range parseHeaders(headBlock) {
		if strings.EqualFold(h.name, "From") {
			if a, err := mail.ParseAddress(strings.TrimSpace(h.value)); err == nil {
				return domainOf(a.Address)
			}
		}
	}
	return ""
}

func domainOf(addr string) string {
	addr = strings.ToLower(strings.TrimSpace(addr))
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return strings.TrimSpace(addr[i+1:])
	}
	return ""
}

// String 返回可存入 Mail.AuthResults 的简短描述。
func (r Result) String() string {
	return fmt.Sprintf("spf=%s; dkim=%s; dmarc=%s", r.SPF, r.DKIM, r.DMARC)
}
