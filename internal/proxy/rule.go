package proxy

import (
	"net/url"
	"regexp"
)

// RuleResult contains the extraction result from a rule.
type RuleResult struct {
	Algo string
	Hash string
}

// Rule defines a function for matching URLs to CAS content.
// It returns a RuleResult if matched, or nil if not.
type Rule func(*url.URL) *RuleResult

// NewRegexRule creates a Rule that matches requests using a regular expression.
// It expects the regex to extract the hash.
func NewRegexRule(regex *regexp.Regexp, algo string) Rule {
	return func(u *url.URL) *RuleResult {
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

		if h, ok := result["hash"]; ok {
			return &RuleResult{Algo: algo, Hash: h}
		}

		// Fallback: use the first capturing group
		if len(matches) > 1 {
			return &RuleResult{Algo: algo, Hash: matches[1]}
		}

		return nil
	}
}
