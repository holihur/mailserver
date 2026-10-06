package sieve

import (
	"strconv"
	"strings"
)

// ---------- 执行 ----------

type interp struct {
	ctx     Context
	acts    []Action
	stopped bool
}

func (in *interp) exec(cmds []command) {
	for _, c := range cmds {
		if in.stopped {
			return
		}
		in.execOne(c)
	}
}

func (in *interp) execOne(c command) {
	switch c.name {
	case "require", "if":
		if c.name == "if" {
			cc := &c
			for cc != nil {
				if cc.test != nil {
					if in.eval(cc.test) {
						in.exec(cc.block)
						return
					}
				} else if cc.name == "else" {
					in.exec(cc.block)
					return
				}
				cc = cc.next
			}
		}
	case "fileinto":
		if len(c.args) > 0 {
			in.acts = append(in.acts, Action{"fileinto", c.args[0]})
		}
	case "redirect":
		if len(c.args) > 0 {
			in.acts = append(in.acts, Action{"redirect", c.args[0]})
		}
	case "discard":
		in.acts = append(in.acts, Action{"discard", ""})
	case "keep":
		in.acts = append(in.acts, Action{"keep", ""})
	case "addflag", "setflag":
		for _, a := range c.args {
			in.acts = append(in.acts, Action{"addflag", a})
		}
	case "reject", "ereject":
		arg := ""
		if len(c.args) > 0 {
			arg = c.args[len(c.args)-1]
		}
		in.acts = append(in.acts, Action{"reject", arg})
	case "vacation":
		in.acts = append(in.acts, Action{"vacation", vacationText(&c)})
	case "stop":
		in.stopped = true
	}
}

// vacationText 取 vacation 的正文（最后一个字符串参数，忽略 :days/:subject 等选项）。
func vacationText(c *command) string {
	if len(c.args) == 0 {
		return ""
	}
	return c.args[len(c.args)-1]
}

func (in *interp) eval(t *test) bool {
	switch t.name {
	case "true":
		return true
	case "false":
		return false
	case "allof":
		for _, s := range t.subs {
			if !in.eval(s) {
				return false
			}
		}
		return true
	case "anyof":
		for _, s := range t.subs {
			if in.eval(s) {
				return true
			}
		}
		return false
	case "not":
		if len(t.subs) > 0 {
			return !in.eval(t.subs[0])
		}
		return false
	case "exists":
		if len(t.args) < 1 {
			return false
		}
		_, ok := in.ctx.Headers[strings.ToLower(t.args[0])]
		return ok
	case "header":
		if len(t.args) < 2 {
			return false
		}
		return matchHeader(t.tags, in.ctx.Headers[strings.ToLower(t.args[0])], t.args[1])
	case "address":
		if len(t.args) < 2 {
			return false
		}
		return matchAddress(t.tags, in.ctx.Headers[strings.ToLower(t.args[0])], t.args[1])
	case "size":
		if len(t.args) < 1 {
			return false
		}
		n, _ := strconv.Atoi(t.args[0])
		for _, tg := range t.tags {
			if tg == ":over" {
				return in.ctx.Size > n
			}
			if tg == ":under" {
				return in.ctx.Size < n
			}
		}
	}
	return false
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if t == want {
			return true
		}
	}
	return false
}

func matchHeader(tags, vals []string, want string) bool {
	contains := hasTag(tags, ":contains")
	for _, v := range vals {
		if contains {
			if strings.Contains(strings.ToLower(v), strings.ToLower(want)) {
				return true
			}
		} else if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func matchAddress(tags, vals []string, want string) bool {
	contains := hasTag(tags, ":contains")
	domainOnly := hasTag(tags, ":domain")
	for _, v := range vals {
		for _, addr := range extractAddrs(v) {
			target := addr
			if domainOnly {
				if i := strings.LastIndex(addr, "@"); i >= 0 {
					target = addr[i+1:]
				}
			}
			if contains {
				if strings.Contains(strings.ToLower(target), strings.ToLower(want)) {
					return true
				}
			} else if strings.EqualFold(target, want) {
				return true
			}
		}
	}
	return false
}

func extractAddrs(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if a, b := strings.Index(part, "<"), strings.Index(part, ">"); a >= 0 && b > a {
			out = append(out, strings.TrimSpace(part[a+1:b]))
			continue
		}
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
