package proxy

import (
	"net/http"
	"regexp"
)

// Rule defines an interface for matching HTTP requests to CAS content.
type Rule interface {
	// Match checks if the request matches the rule.
	// If it matches, it returns the algorithm, the hash, and true.
	Match(req *http.Request) (algo, hash string, match bool)
}

// RegexRule matches requests using a regular expression.
// It expects the regex to extract the hash.
type RegexRule struct {
	Regex *regexp.Regexp
	Algo  string
}

func (r *RegexRule) Match(req *http.Request) (string, string, bool) {
	url := req.URL.String()
	matches := r.Regex.FindStringSubmatch(url)
	if matches == nil {
		return "", "", false
	}

	// Try to find a named group "hash"
	result := make(map[string]string)
	for i, name := range r.Regex.SubexpNames() {
		if i != 0 && name != "" {
			result[name] = matches[i]
		}
	}

	if h, ok := result["hash"]; ok {
		return r.Algo, h, true
	}

	// Fallback: use the first capturing group
	if len(matches) > 1 {
		return r.Algo, matches[1], true
	}

	return "", "", false
}
