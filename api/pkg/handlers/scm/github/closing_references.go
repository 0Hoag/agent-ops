package github

import (
	"regexp"
	"strconv"
	"strings"
)

var refPattern = regexp.MustCompile(`(?i)\b(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+(?:https://github\.com/([\w.-]+/[\w.-]+)/issues/([0-9]+)|([\w.-]+/[\w.-]+)#([0-9]+)|#([0-9]+))`)
var negatedRefPattern = regexp.MustCompile(`(?i)\b(?:do\s+not|don't|never)\s+(?:close[sd]?|fix(?:e[sd])?|resolve[sd]?)\s+(?:https://github\.com/([\w.-]+/[\w.-]+)/issues/([0-9]+)|([\w.-]+/[\w.-]+)#([0-9]+)|#([0-9]+))`)

func closingIssues(body, repo string) []int {
	body = negatedRefPattern.ReplaceAllString(body, "")
	seen := map[int]bool{}
	result := []int{}
	for _, match := range refPattern.FindAllStringSubmatch(body, -1) {
		refRepo, num := match[1], match[2]
		if refRepo == "" {
			refRepo, num = match[3], match[4]
		}
		if refRepo != "" && !strings.EqualFold(refRepo, repo) {
			continue
		}
		if num == "" {
			num = match[5]
		}
		n, err := strconv.Atoi(num)
		if err == nil && !seen[n] {
			seen[n] = true
			result = append(result, n)
		}
	}
	return result
}
