package sieve

import (
	"fmt"
	"strings"
)

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
