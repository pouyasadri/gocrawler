package parser

import (
	"io"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// PageData holds extracted information from a web page.
type PageData struct {
	URL   string   `json:"url"`
	Title string   `json:"title"`
	H1    string   `json:"h1"`
	Links []string `json:"links"`
}

// ParsePage parses HTML and returns a PageData struct with links and metadata.
// baseStr is the page URL used to resolve relative links.
func ParsePage(baseStr string, r io.Reader) (*PageData, error) {
	base, err := url.Parse(baseStr)
	if err != nil {
		return nil, err
	}

	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}

	data := &PageData{URL: baseStr}
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "a" {
				for _, attr := range n.Attr {
					if attr.Key == "href" {
						h := strings.TrimSpace(attr.Val)
						if h == "" || strings.HasPrefix(h, "javascript:") {
							continue
						}
						u, err := base.Parse(h)
						if err == nil {
							data.Links = append(data.Links, u.String())
						}
						break
					}
				}
			} else if n.Data == "title" && data.Title == "" {
				// Title is usually the first text child of title tag
				if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
					data.Title = strings.TrimSpace(n.FirstChild.Data)
				}
			} else if n.Data == "h1" && data.H1 == "" {
				// H1 text might be nested or direct
				data.H1 = extractText(n)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return data, nil
}

// extractText extracts visible text from a node and its children
func extractText(n *html.Node) string {
	var sb strings.Builder
	var f func(*html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return strings.TrimSpace(sb.String())
}

// ExtractLinks is deprecated, use ParsePage instead.
// Kept for backward compatibility if needed, but we essentially removed it.
// If you need it, you can wrap ParsePage.
func ExtractLinks(baseStr string, r io.Reader) ([]string, error) {
	data, err := ParsePage(baseStr, r)
	if err != nil {
		return nil, err
	}
	return data.Links, nil
}
