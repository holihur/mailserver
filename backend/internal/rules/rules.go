// Package rules 实现基于 CEL（Common Expression Language）的收信规则引擎。
// 规则表达式返回 bool，命中后决定邮件投递到哪个文件夹（默认 trash）。
// 支持两个作用域：整站规则（UserID=0，管理员维护）与用户级规则（UserID>0）。
package rules

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"mailserver/internal/model"

	"github.com/google/cel-go/cel"
	"gorm.io/gorm"
)

// Input 规则可访问的邮件字段。
type Input struct {
	From        string
	To          string
	Cc          string
	Bcc         string
	Subject     string
	Body        string
	Size        int
	Attachments int
}

// AllowedFolders 规则可投递的目标文件夹（sent 由系统管理，不允许规则指定）。
var AllowedFolders = map[string]bool{"inbox": true, "draft": true, "trash": true}

// AllowedActions 支持的命中动作。
var AllowedActions = map[string]bool{"trash": true, "move": true, "forward": true}

var (
	envOnce sync.Once
	env     *cel.Env
	envErr  error

	progMu sync.RWMutex
	progs  = map[string]cel.Program{}
)

func celEnv() (*cel.Env, error) {
	envOnce.Do(func() {
		env, envErr = cel.NewEnv(
			cel.Variable("from", cel.StringType),
			cel.Variable("to", cel.StringType),
			cel.Variable("cc", cel.StringType),
			cel.Variable("bcc", cel.StringType),
			cel.Variable("subject", cel.StringType),
			cel.Variable("body", cel.StringType),
			cel.Variable("size", cel.IntType),
			cel.Variable("attachments", cel.IntType),
		)
	})
	return env, envErr
}

func program(expr string) (cel.Program, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, fmt.Errorf("表达式不能为空")
	}
	progMu.RLock()
	p, ok := progs[expr]
	progMu.RUnlock()
	if ok {
		return p, nil
	}
	e, err := celEnv()
	if err != nil {
		return nil, err
	}
	ast, iss := e.Compile(expr)
	if iss != nil && iss.Err() != nil {
		return nil, iss.Err()
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("表达式必须返回布尔值（bool）")
	}
	p, err = e.Program(ast)
	if err != nil {
		return nil, err
	}
	progMu.Lock()
	progs[expr] = p
	progMu.Unlock()
	return p, nil
}

// Validate 校验表达式（保存前/测试用）。
func Validate(expr string) error {
	_, err := program(expr)
	return err
}

// Match 求值表达式，返回是否命中。
func Match(expr string, in Input) (bool, error) {
	p, err := program(expr)
	if err != nil {
		return false, err
	}
	out, _, err := p.Eval(map[string]any{
		"from": in.From, "to": in.To, "cc": in.Cc, "bcc": in.Bcc,
		"subject": in.Subject, "body": in.Body,
		"size": int64(in.Size), "attachments": int64(in.Attachments),
	})
	if err != nil {
		return false, err
	}
	b, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("表达式未返回布尔值")
	}
	return b, nil
}

// Decision 命中结果。
type Decision struct {
	Matched bool     `json:"matched"`
	Action  string   `json:"action"` // trash | move | forward
	Folder  string   `json:"folder"`
	Forward []string `json:"forward"`
	Rule    string   `json:"rule"`
	RuleID  uint     `json:"rule_id"`
	Scope   string   `json:"scope"` // site | user
}

// Evaluate 按「整站优先、用户其次；同级按优先级降序」评估规则，返回首个命中。
// 这样管理员策略拥有最高优先权，用户规则用于个性化。
func Evaluate(list []model.MailRule, in Input) Decision {
	ordered := make([]model.MailRule, 0, len(list))
	for _, r := range list {
		if r.Enabled {
			ordered = append(ordered, r)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		si, sj := scopeRank(ordered[i]), scopeRank(ordered[j])
		if si != sj {
			return si < sj // site(0) 在前
		}
		if ordered[i].Priority != ordered[j].Priority {
			return ordered[i].Priority > ordered[j].Priority
		}
		return ordered[i].ID < ordered[j].ID
	})
	for _, r := range ordered {
		ok, err := Match(r.Expression, in)
		if err != nil || !ok {
			continue
		}
		return Decision{
			Matched: true,
			Action:  actionName(r),
			Folder:  actionFolder(r),
			Forward: forwardTargets(r),
			Rule:    r.Name,
			RuleID:  r.ID,
			Scope:   scopeName(r),
		}
	}
	return Decision{}
}

func scopeRank(r model.MailRule) int {
	if r.UserID == 0 {
		return 0
	}
	return 1
}

func scopeName(r model.MailRule) string {
	if r.UserID == 0 {
		return "site"
	}
	return "user"
}

// actionFolder 依据动作决定目标文件夹。
func actionFolder(r model.MailRule) string {
	if r.Action == "move" && r.Folder != "" {
		return r.Folder
	}
	// forward 会在转发的同时保留一份到收件箱
	return "trash"
}

func actionName(r model.MailRule) string {
	switch r.Action {
	case "move", "forward":
		return r.Action
	default:
		return "trash"
	}
}

// forwardTargets 解析转发目标（仅 action=forward 有效）。
func forwardTargets(r model.MailRule) []string {
	if r.Action != "forward" {
		return nil
	}
	return SplitTargets(r.ForwardTo)
}

// SplitTargets 解析逗号/分号/空白分隔的地址列表（去重、小写）。
func SplitTargets(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	seen := map[string]bool{}
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.ToLower(strings.TrimSpace(f))
		if f == "" || !strings.Contains(f, "@") || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
}

// Load 加载整站规则 + 指定用户的规则（供收信时评估）。
func Load(db *gorm.DB, userID uint) []model.MailRule {
	var rs []model.MailRule
	db.Where("user_id = 0 OR user_id = ?", userID).Find(&rs)
	return rs
}

// Apply 评估规则并返回目标文件夹（未命中返回 fallback，通常 "inbox"）。
func Apply(db *gorm.DB, userID uint, in Input, fallback string) Decision {
	d := Evaluate(Load(db, userID), in)
	if !d.Matched {
		return Decision{Folder: fallback}
	}
	return d
}
