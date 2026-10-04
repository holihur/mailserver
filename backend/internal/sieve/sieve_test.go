package sieve

import "testing"

func run(t *testing.T, script string, hdr map[string]string, size int) Result {
	t.Helper()
	p, err := Compile(script)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	return p.Execute(NewContext(hdr, size))
}

func TestFileInto(t *testing.T) {
	r := run(t, `require ["fileinto"];
if header :contains "Subject" "promo" { fileinto "Trash"; }`,
		map[string]string{"Subject": "Big PROMO here"}, 100)
	if len(r.Actions) != 1 || r.Actions[0].Type != "fileinto" || r.Actions[0].Arg != "Trash" {
		t.Fatalf("%+v", r.Actions)
	}
}

func TestElseChain(t *testing.T) {
	script := `if header :is "From" "a@x.com" { fileinto "A"; }
elsif header :is "From" "b@x.com" { fileinto "B"; }
else { keep; }`
	r := run(t, script, map[string]string{"From": "b@x.com"}, 1)
	if len(r.Actions) != 1 || r.Actions[0].Arg != "B" {
		t.Fatalf("%+v", r.Actions)
	}
	r = run(t, script, map[string]string{"From": "c@x.com"}, 1)
	if len(r.Actions) != 1 || r.Actions[0].Type != "keep" {
		t.Fatalf("%+v", r.Actions)
	}
}

func TestSizeAndLogic(t *testing.T) {
	r := run(t, `if allof (size :over 1000, not header :contains "Subject" "ok") { discard; }`,
		map[string]string{"Subject": "no"}, 2000)
	if len(r.Actions) != 1 || r.Actions[0].Type != "discard" {
		t.Fatalf("%+v", r.Actions)
	}
}

func TestRedirectAndStop(t *testing.T) {
	r := run(t, `redirect "d@y.com"; stop; discard;`, nil, 1)
	if len(r.Actions) != 1 || r.Actions[0].Type != "redirect" || r.Actions[0].Arg != "d@y.com" {
		t.Fatalf("%+v", r.Actions)
	}
}

func TestAddressDomain(t *testing.T) {
	r := run(t, `if address :domain "From" "spam.com" { discard; }`,
		map[string]string{"From": "x@spam.com"}, 1)
	if len(r.Actions) != 1 || r.Actions[0].Type != "discard" {
		t.Fatalf("%+v", r.Actions)
	}
}

func TestAnyOfExists(t *testing.T) {
	r := run(t, `if anyof (exists "X-Spam", header :contains "Subject" "viagra") { fileinto "Junk"; }`,
		map[string]string{"X-Spam": "yes"}, 1)
	if len(r.Actions) != 1 || r.Actions[0].Arg != "Junk" {
		t.Fatalf("%+v", r.Actions)
	}
}

func TestCompileError(t *testing.T) {
	if _, err := Compile(`if header :contains "Subject" {`); err == nil {
		t.Fatal("未闭合应报错")
	}
	if _, err := Compile(`fileinto "x" @`); err == nil {
		t.Fatal("非法字符应报错")
	}
}

func TestCompileErrors(t *testing.T) {
	cases := []string{
		`if header :contains "Subject" {`, // 缺 }
		`fileinto "x" @`,                  // 非法字符
		`}`,                               // 多余 }
		`if`,                              // 缺测试
		`if header :contains "a" "b"`,     // 缺 block
		`if true { keep`,                  // 未闭合块
	}
	for _, s := range cases {
		if _, err := Compile(s); err == nil {
			t.Errorf("应报错: %q", s)
		}
	}
}

func TestLexFeatures(t *testing.T) {
	// 注释 + 字符串列表 + 标签
	r := run(t, "# a comment\nrequire [\"fileinto\", \"imap4flags\"];\nif header :contains \"Subject\" \"x\" { fileinto \"Junk\"; }",
		map[string]string{"Subject": "x"}, 1)
	if len(r.Actions) != 1 || r.Actions[0].Arg != "Junk" {
		t.Fatalf("%+v", r.Actions)
	}
}

func TestFlagsAndNoArgs(t *testing.T) {
	r := run(t, `setflag "\\Seen"; addflag "\\Flagged"; fileinto; redirect;`, nil, 1)
	// setflag + addflag 共 2 个动作；fileinto/redirect 无参数不产生动作
	if len(r.Actions) != 2 || r.Actions[0].Type != "addflag" {
		t.Fatalf("%+v", r.Actions)
	}
}

func TestEvalEdges(t *testing.T) {
	// size 无 :over/:under；exists 无参数；header/address 参数不足；未知测试
	cases := []string{
		`if size "100" { discard; }`,
		`if exists { discard; }`,
		`if header "Subject" { discard; }`,
		`if address "From" { discard; }`,
		`if unknown "x" "y" { discard; }`,
		`if size :under 5 { discard; }`,
	}
	hdr := map[string]string{"Subject": "hello", "From": "a@b.c"}
	for _, s := range cases {
		r := run(t, s, hdr, 10)
		if len(r.Actions) != 0 {
			t.Errorf("%q 不应产生动作: %+v", s, r.Actions)
		}
	}
}

func TestAddressForms(t *testing.T) {
	// :contains 与 <...> 形式
	r := run(t, `if address :contains "To" "example" { discard; }`,
		map[string]string{"To": "Bob <bob@example.com>"}, 1)
	if len(r.Actions) != 1 {
		t.Fatalf(":contains <...>: %+v", r.Actions)
	}
	// :is 精确匹配 <...>
	r = run(t, `if address :is "To" "bob@example.com" { fileinto "B"; }`,
		map[string]string{"To": "Bob <bob@example.com>"}, 1)
	if len(r.Actions) != 1 || r.Actions[0].Arg != "B" {
		t.Fatalf(":is <...>: %+v", r.Actions)
	}
}

func TestTrueFalse(t *testing.T) {
	r := run(t, `if false { discard; } if true { keep; }`, nil, 1)
	if len(r.Actions) != 1 || r.Actions[0].Type != "keep" {
		t.Fatalf("%+v", r.Actions)
	}
}

func TestNotParen(t *testing.T) {
	r := run(t, `if not (header :contains "Subject" "x") { keep; }`,
		map[string]string{"Subject": "y"}, 1)
	if len(r.Actions) != 1 || r.Actions[0].Type != "keep" {
		t.Fatalf("%+v", r.Actions)
	}
}

func TestDefaultBranch(t *testing.T) {
	// fileinto 后跟 ident（非 tag/str/num）→ 解析成独立命令；未知命令现在编译期报错，不再静默跳过
	if _, err := Compile(`fileinto foo; keep;`); err == nil {
		t.Fatal("未知命令 foo 应编译报错")
	}
}

func TestRejectVacationAndValidate(t *testing.T) {
	p, err := Compile(`require ["reject"]; if header :contains "Subject" "x" { reject "no thanks"; }`)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	res := p.Execute(NewContext(map[string]string{"Subject": "x"}, 10))
	if len(res.Actions) != 1 || res.Actions[0].Type != "reject" || res.Actions[0].Arg != "no thanks" {
		t.Fatalf("reject 动作不正确: %+v", res.Actions)
	}

	pv, err := Compile(`require ["vacation"]; vacation :days 7 "out of office";`)
	if err != nil {
		t.Fatalf("compile vacation: %v", err)
	}
	res = pv.Execute(NewContext(nil, 1))
	if len(res.Actions) != 1 || res.Actions[0].Type != "vacation" || res.Actions[0].Arg != "out of office" {
		t.Fatalf("vacation 动作不正确: %+v", res.Actions)
	}

	if _, err := Compile(`bogus_action "x";`); err == nil {
		t.Fatal("未知动作应编译报错")
	}
}

func FuzzSieve(f *testing.F) {
	seeds := []string{
		`if header :contains "Subject" "x" { fileinto "Trash"; }`,
		`require ["fileinto"];`,
		`if allof (size :over 1, not exists "X") { discard; }`,
		`if address :domain "From" "a.com" { redirect "b@c.d"; stop; }`,
		`if anyof (true, false) { keep; } else { discard; }`,
		``,
		`if`,
		`fileinto "x" @`,
		`{`,
		`}`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		// 目标：任意输入都不 panic；编译成功则可安全执行
		p, err := Compile(s)
		if err == nil {
			_ = p.Execute(NewContext(map[string]string{"Subject": "x", "From": "a@b.c"}, 42))
		}
	})
}

func TestMoreEdges(t *testing.T) {
	// 解析错误 / 边界：只要求不 panic
	scripts := []string{
		`fileinto "abc`,               // 未闭合字符串
		`if true { if }`,              // 块内 parseCommands 出错
		`fileinto "x"`,                // EOF 无分号
		`fileinto "x" { }`,            // 非 if 命令带块
		`fileinto "x" { if }`,         // 块解析出错
		`if true { } elsif }`,         // elsif 测试出错
		`if true { } elsif true`,      // elsif 块出错
		`if allof ( } { }`,            // allof 子测试出错
		`if not ( } { }`,              // not( 子测试出错
		`if not } { }`,                // not 子测试出错
		`if size { discard; }`,        // size 无参数
		`require ["a", b, "c"];`,      // 列表含非字符串
		`fileinto :create "Archive";`, // 命令带 tag
		`if allof } { }`,              // allof 缺 (
		`if allof (true } { }`,        // allof 缺 )
	}
	for _, s := range scripts {
		p, err := Compile(s)
		if err == nil {
			_ = p.Execute(NewContext(map[string]string{"Subject": "x"}, 1))
		}
	}
	if run(t, `if allof (false, true) { discard; }`, nil, 1).Actions != nil {
		t.Fatal("allof 短路应不命中")
	}
	if run(t, `if anyof (false, false) { discard; }`, nil, 1).Actions != nil {
		t.Fatal("anyof 全 false 应不命中")
	}
}

func TestEvalNotNoSubs(t *testing.T) {
	in := &interp{ctx: NewContext(nil, 1)}
	if in.eval(&test{name: "not"}) {
		t.Fatal("无子测试的 not 应为 false")
	}
}
