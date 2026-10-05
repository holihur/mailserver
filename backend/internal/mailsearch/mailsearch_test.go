package mailsearch

import (
	"testing"
	"time"
)

// #14：Gmail 子集语法解析。
func TestParse(t *testing.T) {
	q := Parse(`from:alice subject:"hello world" after:2024-01-02 before:2024-02-01 has:attachment 报表`)
	if len(q.Terms) != 3 {
		t.Fatalf("want 3 terms, got %d: %+v", len(q.Terms), q.Terms)
	}
	if q.Terms[0].Field != "from" || q.Terms[0].Value != "alice" {
		t.Fatalf("from 解析错误: %+v", q.Terms[0])
	}
	if q.Terms[1].Field != "subject" || q.Terms[1].Value != "hello world" {
		t.Fatalf("subject 解析错误: %+v", q.Terms[1])
	}
	if q.Terms[2].Field != "" || q.Terms[2].Value != "报表" {
		t.Fatalf("裸词解析错误: %+v", q.Terms[2])
	}
	if !q.HasAttachment {
		t.Fatalf("has:attachment 未识别")
	}
	wantAfter := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	if !q.After.Equal(wantAfter) {
		t.Fatalf("after 解析错误: %v", q.After)
	}
	wantBefore := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	if !q.Before.Equal(wantBefore) {
		t.Fatalf("before 解析错误: %v", q.Before)
	}
	if Parse("").IsEmpty() != true {
		t.Fatalf("空查询应为 empty")
	}
}

// #14：粗筛 SQL 只做 OR 扩大候选集，且对 LIKE 通配符转义。
func TestCoarseFilter(t *testing.T) {
	expr, args := CoarseFilter([]Term{{Field: "from", Value: "a%b_c"}})
	if expr == "" || len(args) != 1 {
		t.Fatalf("coarse filter 生成失败: %q %v", expr, args)
	}
	if args[0] != `%a\%b\_c%` {
		t.Fatalf("通配符未转义: %v", args[0])
	}
	expr, args = CoarseFilter([]Term{{Value: "x"}, {Field: "body", Value: "y"}})
	if expr == "" || len(args) != 5 {
		t.Fatalf("应生成 1+4 个参数: %q %v", expr, args)
	}
	if e, _ := CoarseFilter(nil); e != "" {
		t.Fatalf("无词时应返回空")
	}
}
