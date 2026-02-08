package parser

import (
	"strings"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	htmlContent := `
		<html>
			<body>
				<a href="/foo">Foo</a>
				<a href="http://example.com/bar">Bar</a>
				<a href="javascript:void(0)">Ignore</a>
				<a href="">Empty</a>
			</body>
		</html>
	`
	base := "http://example.com"
	r := strings.NewReader(htmlContent)

	links, err := ExtractLinks(base, r)
	if err != nil {
		t.Fatalf("ExtractLinks failed: %v", err)
	}

	expected := []string{
		"http://example.com/foo",
		"http://example.com/bar",
	}

	if len(links) != len(expected) {
		t.Errorf("expected %d links, got %d", len(expected), len(links))
	}

	for i, link := range links {
		if link != expected[i] {
			t.Errorf("expected link %d to be %s, got %s", i, expected[i], link)
		}
	}
}
