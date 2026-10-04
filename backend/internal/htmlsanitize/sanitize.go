// Package htmlsanitize 对邮件 HTML 正文做「严格白名单」清洗：
//   - 只保留安全标签，其余标签（保留文字）展开或整体丢弃；
//   - 只保留安全属性，去掉 on* 事件、style、以及 javascript:/data:text/html 等危险 URL；
//   - 远程图片改写为 data-blocked-src（前端经代理按需加载）；
//   - 去掉注释节点。
package htmlsanitize

import (
	"strings"

	"golang.org/x/net/html"
)

// 整体丢弃（连同内容）的标签。
var dropTags = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true, "embed": true,
	"link": true, "meta": true, "base": true, "form": true, "input": true,
	"button": true, "textarea": true, "select": true, "option": true,
	"frame": true, "frameset": true, "applet": true, "template": true,
	"svg": true, "math": true, "noscript": true, "title": true,
}

// 允许保留的标签（其余展开为文字）。
var allowedTags = map[string]bool{
	"html": true, "head": true, "body": true, "div": true, "span": true, "p": true,
	"br": true, "hr": true, "a": true, "b": true, "strong": true, "i": true, "em": true,
	"u": true, "s": true, "strike": true, "del": true, "ins": true, "mark": true,
	"small": true, "big": true, "sub": true, "sup": true, "font": true, "center": true,
	"ul": true, "ol": true, "li": true, "dl": true, "dt": true, "dd": true,
	"blockquote": true, "pre": true, "code": true, "tt": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true,
	"td": true, "th": true, "caption": true, "colgroup": true, "col": true,
	"img": true,
}

// 各标签允许的属性；"*" 为全局允许。
var allowedAttrs = map[string]map[string]bool{
	"*":     {"title": true, "dir": true, "lang": true},
	"a":     {"href": true},
	"img":   {"src": true, "alt": true, "width": true, "height": true},
	"table": {"border": true, "cellpadding": true, "cellspacing": true, "width": true, "height": true, "align": true},
	"td":    {"colspan": true, "rowspan": true, "align": true, "valign": true, "width": true, "height": true},
	"th":    {"colspan": true, "rowspan": true, "align": true, "valign": true, "width": true, "height": true},
	"col":   {"span": true, "width": true},
	"ol":    {"start": true, "type": true},
	"font":  {"color": true, "face": true, "size": true},
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
		switch {
		case c.Type == html.CommentNode:
			n.RemoveChild(c)
		case c.Type == html.ElementNode && dropTags[c.Data]:
			n.RemoveChild(c)
		case c.Type == html.ElementNode && !allowedTags[c.Data]:
			unwrap(c) // 未知标签：保留文字，去掉标签
		case c.Type == html.ElementNode:
			cleanAttrs(c)
			clean(c)
		default:
			clean(c)
		}
		c = next
	}
}

// unwrap 用子节点替换自身。
func unwrap(n *html.Node) {
	parent := n.Parent
	if parent == nil {
		return
	}
	for c := n.FirstChild; c != nil; {
		next := c.NextSibling
		parent.InsertBefore(c, n)
		c = next
	}
	parent.RemoveChild(n)
}

func cleanAttrs(n *html.Node) {
	allow := allowedAttrs[n.Data]
	keep := n.Attr[:0]
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		if !allowedAttrs["*"][key] && !allow[key] {
			continue
		}
		if key == "href" || key == "src" {
			v := strings.ToLower(strings.TrimSpace(a.Val))
			if strings.HasPrefix(v, "javascript:") || strings.HasPrefix(v, "vbscript:") ||
				strings.HasPrefix(v, "data:text/html") || strings.HasPrefix(v, "data:image/svg") {
				continue
			}
			// 远程图片：改写为 data-blocked-src，前端点「显示图片」时经代理加载
			if n.Data == "img" && key == "src" &&
				(strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://")) {
				a.Key = "data-blocked-src"
			}
		}
		keep = append(keep, a)
	}
	n.Attr = keep
}
