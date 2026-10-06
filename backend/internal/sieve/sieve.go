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

// 支持的动作与能力。
var actionNames = map[string]bool{
	"keep": true, "discard": true, "fileinto": true, "redirect": true,
	"addflag": true, "setflag": true, "stop": true,
	"reject": true, "ereject": true, "vacation": true,
}
var controlNames = map[string]bool{"require": true, "if": true, "elsif": true, "else": true}

// Capabilities 返回脚本引擎实际支持的能力（供 ManageSieve 通告，避免“吹牛不兼现”）。
func Capabilities() []string {
	return []string{"fileinto", "reject", "ereject", "vacation", "imap4flags"}
}

// Compile 解析脚本并校验未知命令（未知直接报错，不再静默跳过）。
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
	if err := validateBlock(cmds); err != nil {
		return nil, err
	}
	return &Program{cmds: cmds}, nil
}

func validateBlock(cmds []command) error {
	for i := range cmds {
		if err := validate(&cmds[i]); err != nil {
			return err
		}
	}
	return nil
}

func validate(c *command) error {
	for ; c != nil; c = c.next {
		if !actionNames[c.name] && !controlNames[c.name] {
			return fmt.Errorf("不支持的命令/动作: %s", c.name)
		}
		if err := validateBlock(c.block); err != nil {
			return err
		}
	}
	return nil
}

// Execute 执行脚本。
func (p *Program) Execute(ctx Context) Result {
	in := &interp{ctx: ctx}
	in.exec(p.cmds)
	return Result{Actions: in.acts}
}
