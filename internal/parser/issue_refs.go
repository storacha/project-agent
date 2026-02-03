package parser

import (
	"regexp"
	"strconv"
	"strings"
)

// IssueReference represents a reference to a GitHub issue
type IssueReference struct {
	Owner      string // Repository owner (e.g., "storacha")
	Repo       string // Repository name (e.g., "guppy")
	Number     int    // Issue number
	IsExplicit bool   // True if referenced with keywords like "fixes", "closes"
}

var (
	// Match: fixes #123, closes #456, resolves #789
	keywordPattern = regexp.MustCompile(`(?i)\b(fix|fixes|fixed|close|closes|closed|resolve|resolves|resolved)\s+#(\d+)`)

	// Match: #123
	simplePattern = regexp.MustCompile(`\B#(\d+)\b`)

	// Match: storacha/repo#123, owner/repo#456
	crossRepoPattern = regexp.MustCompile(`\b([a-zA-Z0-9_-]+)/([a-zA-Z0-9_-]+)#(\d+)\b`)

	// Match: https://github.com/storacha/repo/issues/123
	urlPattern = regexp.MustCompile(`https?://github\.com/([a-zA-Z0-9_-]+)/([a-zA-Z0-9_-]+)/issues/(\d+)`)
)

// ParseIssueReferences extracts all issue references from PR title and body
func ParseIssueReferences(title, body, defaultOwner, defaultRepo string) []IssueReference {
	text := title + "\n" + body
	refs := make(map[string]IssueReference) // Use map to deduplicate

	// Parse keyword references (fixes #123)
	matches := keywordPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) >= 3 {
			num, err := strconv.Atoi(match[2])
			if err == nil {
				key := makeKey(defaultOwner, defaultRepo, num)
				refs[key] = IssueReference{
					Owner:      defaultOwner,
					Repo:       defaultRepo,
					Number:     num,
					IsExplicit: true,
				}
			}
		}
	}

	// Parse cross-repo references (storacha/guppy#123)
	matches = crossRepoPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) >= 4 {
			num, err := strconv.Atoi(match[3])
			if err == nil {
				owner := match[1]
				repo := match[2]
				key := makeKey(owner, repo, num)
				if _, exists := refs[key]; !exists {
					refs[key] = IssueReference{
						Owner:      owner,
						Repo:       repo,
						Number:     num,
						IsExplicit: false,
					}
				}
			}
		}
	}

	// Parse URL references
	matches = urlPattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) >= 4 {
			num, err := strconv.Atoi(match[3])
			if err == nil {
				owner := match[1]
				repo := match[2]
				key := makeKey(owner, repo, num)
				if _, exists := refs[key]; !exists {
					refs[key] = IssueReference{
						Owner:      owner,
						Repo:       repo,
						Number:     num,
						IsExplicit: false,
					}
				}
			}
		}
	}

	// Parse simple references (#123) - do last to not override explicit ones
	matches = simplePattern.FindAllStringSubmatch(text, -1)
	for _, match := range matches {
		if len(match) >= 2 {
			num, err := strconv.Atoi(match[1])
			if err == nil {
				key := makeKey(defaultOwner, defaultRepo, num)
				if _, exists := refs[key]; !exists {
					refs[key] = IssueReference{
						Owner:      defaultOwner,
						Repo:       defaultRepo,
						Number:     num,
						IsExplicit: false,
					}
				}
			}
		}
	}

	// Convert map to slice
	result := make([]IssueReference, 0, len(refs))
	for _, ref := range refs {
		result = append(result, ref)
	}

	return result
}

// makeKey creates a unique key for deduplication
func makeKey(owner, repo string, number int) string {
	return strings.ToLower(owner) + "/" + strings.ToLower(repo) + "#" + strconv.Itoa(number)
}
