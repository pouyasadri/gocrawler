package robotstxt

import (
	"strings"
	"testing"
)

func TestRobotsTxt(t *testing.T) {
	robotsContent := `
User-agent: *
Disallow: /admin
Disallow: /private

User-agent: BadBot
Disallow: /

User-agent: Googlebot
Disallow: /no-google
`

	t.Run("General User Agent", func(t *testing.T) {
		r := strings.NewReader(robotsContent)
		checker := New(r, "MyCrawler")

		tests := []struct {
			path    string
			allowed bool
		}{
			{"/", true},
			{"/public", true},
			{"/admin", false},
			{"/admin/login", false},
			{"/private/data", false},
			{"/no-google", true}, // We are not Googlebot, so we ignore that block (in our simple logic, strictly speaking we should only obey the most specific, but our logic accumulates * and matching. Wait, my logic accumulates from ALL relevant blocks. Let's verify.)
		}

		for _, test := range tests {
			if got := checker.Allowed(test.path); got != test.allowed {
				t.Errorf("Allowed(%q) = %v, want %v", test.path, got, test.allowed)
			}
		}
	})

	t.Run("Specific User Agent", func(t *testing.T) {
		r := strings.NewReader(robotsContent)
		checker := New(r, "BadBot")

		if checker.Allowed("/") {
			t.Error("BadBot should be disallowed from /")
		}
	})
}
