// Package htmlsanitize 对邮件 HTML 正文做安全清洗：移除脚本/样式/框架等危险元素，
// 去掉 on* 事件与 javascript: 链接，并把远程图片改为 data-blocked-src（前端按需经代理加载）。
package htmlsanitize

import (
	"strings"

	"golang.org/x/net/html"
)

var dropTags = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true, "embed": true,
	"link": true, "meta": true, "base": true, "form": true, "input": true,
	"button": true, "textarea": true, "select": true, "frame": true, "frameset": true,
	"applet": true, "template": true, "svg": true, "math": true,
}

var urlAttrs = map[string]bool{
	"href": true, "src": true, "action": true, "formaction": true,
	"background": true, "poster": true, "xlink:href": true, "srcset": true,
}

// Sanitize 返回清洗后的 HTML；空输入返回空串。
func Sanitize(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return ""
	}
	clean(doc)
	var b strings.Builder
	if err := html.Render(&b, doc); err != nil {
		return ""
	}
	return b.String()
}

func clean(n *html.Node) {
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		if c.Type == html.ElementNode && dropTags[c.Data] {
			n.RemoveChild(c)
			c = next
			continue
		}
		if c.Type == html.ElementNode {
			cleanAttrs(c)
		}
		clean(c)
		c = next
	}
}

func cleanAttrs(n *html.Node) {
	keep := n.Attr[:0]
	for _, a := range n.Attr {
		name := strings.ToLower(a.Key)
		if strings.HasPrefix(name, "on") { // on* 事件
			continue
		}
		if name == "style" { // 内联样式可能用于隐藏/钓鱼
			continue
		}
		if name == "srcset" {
			continue
		}
		if urlAttrs[name] {
			v := strings.ToLower(strings.TrimSpace(a.Val))
			if strings.HasPrefix(v, "javascript:") || strings.HasPrefix(v, "vbscript:") ||
				strings.HasPrefix(v, "data:text/html") || strings.HasPrefix(v, "data:image/svg") {
				continue
			}
			// 远程图片：改写为 data-blocked-src，前端点“显示图片”时经代理加载
			if n.Data == "img" && name == "src" &&
				(strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://")) {
				a.Key = "data-blocked-src"
			}
		}
		keep = append(keep, a)
	}
	n.Attr = keep
}
