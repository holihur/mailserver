// Package sieve 实现 Sieve（RFC 5228）的一个实用子集，用于服务端收信过滤。
//
// 支持：
//   - 控制：require / if / elsif / else / stop
//   - 动作：keep / discard / fileinto / redirect / addflag / setflag
//   - 测试：header / address / size / exists / allof / anyof / not / true / false
//   - 匹配：:is / :contains / :domain（address）
package sieve

import (
	"fmt"
	"strconv"
	"strings"
)

// Action 表示脚本执行产生的一个动作。
type Action struct {
	Type string // fileinto | redirect | discard | keep | addflag
	Arg  string
}

// Result 是脚本对一封邮件的处理结果。
type Result struct {
	Actions []Action
}

// Context 是执行脚本时的邮件上下文。
type Context struct {
	Headers map[string][]string
	Size    int
}

// NewContext 用邮件头（单值）与大小构建上下文。
func NewContext(hdr map[string]string, size int) Context {
	h := map[string][]string{}
	for k, v := range hdr {
		h[strings.ToLower(k)] = []string{v}
	}
	return Context{Headers: h, Size: size}
}

// Program 是编译后的脚本。
type Program struct{ cmds []command }

// Compile 解析脚本（不做语义/能力检查）。
func Compile(script string) (*Program, error) {
	toks, err := lex(script)
	if err != nil {
		return nil, err
	}
	p := &parser{t: toks}
	cmds, err := p.parseCommands()
	if err != nil {
		return nil, err
	}
	if p.i < len(p.t) {
		return nil, fmt.Errorf("脚本尾部有多余内容")
	}
	return &Program{cmds: cmds}, nil
}

// Execute 执行脚本。
func (p *Program) Execute(ctx Context) Result {
	in := &interp{ctx: ctx}
	in.exec(p.cmds)
	return Result{Actions: in.acts}
}

// ---------- 词法 ----------

type tok struct{ kind, val string }

func isIdentStart(c byte) bool { return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9') || c == '-' || c == '.'
}

func lex(s string) ([]tok, error) {
	var out []tok
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			i++
		case c == '#':
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '"':
			i++
			var b strings.Builder
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' && i+1 < len(s) {
					i++
					b.WriteByte(s[i])
					i++
					continue
				}
				b.WriteByte(s[i])
				i++
			}
			if i >= len(s) {
				return nil, fmt.Errorf("字符串未闭合")
			}
			i++
			out = append(out, tok{"str", b.String()})
		case c == '{' || c == '}' || c == '(' || c == ')' || c == ',' || c == ';' || c == '[' || c == ']':
			out = append(out, tok{"punct", string(c)})
			i++
		case c == ':':
			j := i + 1
			for j < len(s) && isIdentChar(s[j]) {
				j++
			}
			out = append(out, tok{"tag", strings.ToLower(s[i:j])})
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			out = append(out, tok{"num", s[i:j]})
			i = j
		case isIdentStart(c):
			j := i
			for j < len(s) && isIdentChar(s[j]) {
				j++
			}
			out = append(out, tok{"ident", s[i:j]})
			i = j
		default:
			return nil, fmt.Errorf("非法字符: %q", string(c))
		}
	}
	return out, nil
}

// ---------- 语法 ----------

type command struct {
	name  string
	tags  []string
	args  []string
	test  *test
	block []command
	next  *command
}

type test struct {
	name string
	tags []string
	args []string
	subs []*test
}

type parser struct {
	t []tok
	i int
}

func (p *parser) peek() *tok {
	if p.i < len(p.t) {
		return &p.t[p.i]
	}
	return nil
}
func (p *parser) next() *tok {
	t := p.peek()
	if t != nil {
		p.i++
	}
	return t
}
func (p *parser) expect(val string) error {
	t := p.next()
	if t == nil || t.val != val {
		return fmt.Errorf("期望 %q", val)
	}
	return nil
}

func (p *parser) parseCommands() ([]command, error) {
	var cmds []command
	for {
		t := p.peek()
		if t == nil || (t.kind == "punct" && t.val == "}") {
			break
		}
		c, err := p.parseCommand()
		if err != nil {
			return nil, err
		}
		cmds = append(cmds, c)
	}
	return cmds, nil
}

func (p *parser) parseBlock() ([]command, error) {
	if err := p.expect("{"); err != nil {
		return nil, err
	}
	cmds, err := p.parseCommands()
	if err != nil {
		return nil, err
	}
	if err := p.expect("}"); err != nil {
		return nil, err
	}
	return cmds, nil
}

func (p *parser) parseCommand() (command, error) {
	nameTok := p.next()
	if nameTok == nil || nameTok.kind != "ident" {
		return command{}, fmt.Errorf("期望命令名")
	}
	c := command{name: strings.ToLower(nameTok.val)}

	if c.name == "if" {
		ts, err := p.parseTest()
		if err != nil {
			return c, err
		}
		c.test = ts
		blk, err := p.parseBlock()
		if err != nil {
			return c, err
		}
		c.block = blk
		if err := p.parseChain(&c); err != nil {
			return c, err
		}
		return c, nil
	}

	for {
		n := p.peek()
		if n == nil {
			break
		}
		switch {
		case n.kind == "tag":
			c.tags = append(c.tags, n.val)
			p.next()
		case n.kind == "str" || n.kind == "num":
			c.args = append(c.args, n.val)
			p.next()
		case n.val == "[":
			p.next()
			for {
				x := p.peek()
				if x == nil || x.val == "]" {
					break
				}
				if x.kind == "str" || x.kind == "num" {
					c.args = append(c.args, x.val)
				}
				p.next()
			}
			_ = p.expect("]")
		case n.val == ";":
			p.next()
			return c, nil
		case n.val == "{":
			blk, err := p.parseBlock()
			if err != nil {
				return c, err
			}
			c.block = blk
			return c, nil
		default:
			return c, nil
		}
	}
	return c, nil
}

func (p *parser) parseChain(c *command) error {
	n := p.peek()
	if n == nil || n.kind != "ident" || (n.val != "elsif" && n.val != "else") {
		return nil
	}
	p.next()
	ch := &command{name: strings.ToLower(n.val)}
	if ch.name == "elsif" {
		ts, err := p.parseTest()
		if err != nil {
			return err
		}
		ch.test = ts
	}
	blk, err := p.parseBlock()
	if err != nil {
		return err
	}
	ch.block = blk
	c.next = ch
	return p.parseChain(ch)
}

func (p *parser) parseTest() (*test, error) {
	t := p.next()
	if t == nil || t.kind != "ident" {
		return nil, fmt.Errorf("期望测试名")
	}
	ts := &test{name: strings.ToLower(t.val)}
	switch ts.name {
	case "allof", "anyof":
		if err := p.expect("("); err != nil {
			return nil, err
		}
		for {
			sub, err := p.parseTest()
			if err != nil {
				return nil, err
			}
			ts.subs = append(ts.subs, sub)
			n := p.peek()
			if n != nil && n.val == "," {
				p.next()
				continue
			}
			break
		}
		if err := p.expect(")"); err != nil {
			return nil, err
		}
		return ts, nil
	case "not":
		// RFC 5228: "not" 后直接跟单个 test（也容忍括号写法）
		if n := p.peek(); n != nil && n.val == "(" {
			p.next()
			sub, err := p.parseTest()
			if err != nil {
				return nil, err
			}
			ts.subs = append(ts.subs, sub)
			_ = p.expect(")")
			return ts, nil
		}
		sub, err := p.parseTest()
		if err != nil {
			return nil, err
		}
		ts.subs = append(ts.subs, sub)
		return ts, nil
	case "true", "false":
		return ts, nil
	}
	for {
		n := p.peek()
		if n == nil {
			break
		}
		if n.kind == "tag" {
			ts.tags = append(ts.tags, n.val)
			p.next()
			continue
		}
		if n.kind == "str" || n.kind == "num" {
			ts.args = append(ts.args, n.val)
			p.next()
			continue
		}
		break
	}
	return ts, nil
}

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
	case "stop":
		in.stopped = true
	}
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
