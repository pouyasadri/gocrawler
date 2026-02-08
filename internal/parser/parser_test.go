package parser

import (
	"strings"
	"testing"
)

func TestParsePage(t *testing.T) {
	htmlContent := `
		<html>
			<head>
				<title>  Test Page Title  </title>
			</head>
			<body>
				<h1>  Main Header </h1>
				<a href="/foo">Foo</a>
				<a href="http://example.com/bar">Bar</a>
				<a href="javascript:void(0)">Ignore</a>
				<a href="">Empty</a>
			</body>
		</html>
	`
	base := "http://example.com"
	r := strings.NewReader(htmlContent)

	data, err := ParsePage(base, r)
	if err != nil {
		t.Fatalf("ParsePage failed: %v", err)
	}

	if data.Title != "Test Page Title" {
		t.Errorf("expected title 'Test Page Title', got '%s'", data.Title)
	}
	if data.H1 != "Main Header" {
		t.Errorf("expected h1 'Main Header', got '%s'", data.H1)
	}

	expectedLinks := []string{
		"http://example.com/foo",
		"http://example.com/bar",
	}

	if len(data.Links) != len(expectedLinks) {
		t.Errorf("expected %d links, got %d", len(expectedLinks), len(data.Links))
	}

	for i, link := range data.Links {
		if link != expectedLinks[i] {
			t.Errorf("expected link %d to be %s, got %s", i, expectedLinks[i], link)
		}
	}
}

func TestExtractLinks(t *testing.T) {
	// Deprecation check - wrapper still works
	htmlContent := `<a href="/foo">Foo</a>`
	r := strings.NewReader(htmlContent)
	links, _ := ExtractLinks("http://example.com", r)
	if len(links) != 1 || links[0] != "http://example.com/foo" {
		t.Errorf("ExtractLinks wrapper broken")
	}
}
