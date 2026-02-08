package robotstxt

import (
	"bufio"
	"io"
	"strings"
)

type RobotsChecker struct {
	disallowed []string
}

func New(r io.Reader, userAgent string) *RobotsChecker {
	scanner := bufio.NewScanner(r)
	checker := &RobotsChecker{}

	targetUA := strings.ToLower(userAgent)
	currentUserAgent := ""
	isRelevantUA := false

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(strings.ToLower(parts[0]))
		val := strings.TrimSpace(parts[1])

		if key == "user-agent" {
			currentUserAgent = strings.ToLower(val)
			// check if this UA block applies to us
			// straightforward logic: if it's * or contains our UA string
			// NOTE: real robots.txt logic is more complex (specific wins over wildcard),
			// but for this simple crawler, we'll just accumulate disallows from '*' or matching UA.
			isRelevantUA = (currentUserAgent == "*") || strings.Contains(targetUA, currentUserAgent)
		} else if key == "disallow" && isRelevantUA {
			if val != "" {
				checker.disallowed = append(checker.disallowed, val)
			}
		}
	}
	return checker
}

func (c *RobotsChecker) Allowed(path string) bool {
	for _, pattern := range c.disallowed {
		if strings.HasPrefix(path, pattern) {
			return false
		}
	}
	return true
}
