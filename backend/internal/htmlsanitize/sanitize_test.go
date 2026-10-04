package htmlsanitize

import (
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	in := `<div onclick="evil()"><script>alert(1)</script>` +
		`<p style="color:red">hi <a href="javascript:evil()">x</a></p>` +
		`<img src="https://tracker.example/p.png"><img src="data:image/png;base64,AAA"></div>`
	out := Sanitize(in)
	for _, bad := range []string{"<script", "onclick", "javascript:", "style="} {
		if strings.Contains(out, bad) {
			t.Errorf("应移除 %q，得到: %s", bad, out)
		}
	}
	if !strings.Contains(out, "data-blocked-src") {
		t.Errorf("远程图片应改写为 data-blocked-src: %s", out)
	}
	if !strings.Contains(out, "data:image/png") {
		t.Errorf("内联图片应保留: %s", out)
	}
	if Sanitize("") != "" {
		t.Error("空输入应返回空")
	}
	if Sanitize("<p>ok</p>") == "" {
		t.Error("正常 HTML 不应被清空")
	}
}
