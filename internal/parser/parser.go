package parser

import (
	"io"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// ExtractLinks parses HTML and returns absolute URLs as strings.
// base is the page URL used to resolve relative links.
func ExtractLinks(baseStr string, r io.Reader) ([]string, error) {
	base, err := url.Parse(baseStr)
	if err != nil {
		return nil, err
	}

	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	var out []string
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					h := strings.TrimSpace(attr.Val)
					if h == "" || strings.HasPrefix(h, "javascript:") {
						continue
					}
					u, err := base.Parse(h)
					if err == nil {
						out = append(out, u.String())
					}
					break
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return out, nil
}
