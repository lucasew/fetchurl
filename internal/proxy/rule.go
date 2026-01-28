package proxy

import (
	"context"
	"net/url"
	"regexp"
)

// RuleResult contains the extraction result from a rule.
type RuleResult struct {
	Algo string
	Hash string
}

// Rule defines a function for matching URLs to CAS content.
// It returns a slice of RuleResults ordered by priority, or nil/empty slice if not matched.
type Rule func(context.Context, *url.URL) []RuleResult

// NewRegexRule creates a Rule that matches requests using a regular expression.
// It expects the regex to extract the hash.
func NewRegexRule(regex *regexp.Regexp, algo string) Rule {
	return func(ctx context.Context, u *url.URL) []RuleResult {
		urlString := u.String()
		matches := regex.FindStringSubmatch(urlString)
		if matches == nil {
			return nil
		}

		// Try to find a named group "hash"
		result := make(map[string]string)
		for i, name := range regex.SubexpNames() {
			if i != 0 && name != "" {
				result[name] = matches[i]
			}
		}

		var hash string
		if h, ok := result["hash"]; ok {
			hash = h
		} else if len(matches) > 1 {
			// Fallback: use the first capturing group
			hash = matches[1]
		}

		if hash == "" {
			return nil
		}

		return []RuleResult{{Algo: algo, Hash: hash}}
	}
}
