package ui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// validateSavedFilter recognizes only the GitHub filter terms exercised by
// the read probes. Accepted filters are still evaluated by GitHub, not locally.
func validateSavedFilter(filter string) error {
	terms, err := splitSavedFilterTerms(strings.TrimSpace(filter))
	if err != nil {
		return err
	}
	seen := make(map[string]bool, len(terms))
	classes := make([]string, 0, len(terms))
	for _, term := range terms {
		if err := validateSavedFilterTerm(term); err != nil {
			return err
		}
		if seen[term] {
			return fmt.Errorf("duplicate filter term %q is not supported", term)
		}
		seen[term] = true
		classes = append(classes, savedFilterTermClass(term))
	}
	if len(classes) > 2 {
		return fmt.Errorf("conjunctions of more than two verified terms are not supported")
	}
	if len(classes) == 2 {
		pair := map[string]bool{classes[0]: true, classes[1]: true}
		verified := (pair["status"] && pair["assignee"]) ||
			(pair["negative-status"] && pair["assignee"]) ||
			(pair["iteration"] && pair["status"])
		if !verified {
			return fmt.Errorf("conjunction of these filter terms has not been verified")
		}
	}
	return nil
}

func savedFilterTermClass(term string) string {
	switch {
	case term == "iteration:@current":
		return "iteration"
	case term == "assignee:@me":
		return "assignee"
	case term == "no:status":
		return "no-status"
	case strings.HasPrefix(term, "-status:"):
		return "negative-status"
	default:
		return "status"
	}
}

func splitSavedFilterTerms(filter string) ([]string, error) {
	runes := []rune(filter)
	terms := make([]string, 0, 4)
	for index := 0; index < len(runes); {
		for index < len(runes) && unicode.IsSpace(runes[index]) {
			index++
		}
		if index == len(runes) {
			break
		}
		start := index
		quoted, escaped := false, false
		for index < len(runes) {
			current := runes[index]
			if quoted {
				if escaped {
					escaped = false
				} else if current == '\\' {
					escaped = true
				} else if current == '"' {
					quoted = false
				}
			} else if current == '"' {
				quoted = true
			} else if unicode.IsSpace(current) {
				break
			}
			index++
		}
		if quoted || escaped {
			return nil, fmt.Errorf("unterminated quoted filter value")
		}
		terms = append(terms, string(runes[start:index]))
	}
	return terms, nil
}

func validateSavedFilterTerm(term string) error {
	switch term {
	case "iteration:@current", "assignee:@me", "no:status":
		return nil
	}
	for _, prefix := range []string{"status:", "-status:"} {
		if !strings.HasPrefix(term, prefix) {
			continue
		}
		quoted := strings.TrimPrefix(term, prefix)
		if len(quoted) < 2 || quoted[0] != '"' || quoted[len(quoted)-1] != '"' {
			break
		}
		if strings.ContainsRune(quoted[1:len(quoted)-1], '\\') {
			break
		}
		value, err := strconv.Unquote(quoted)
		if err == nil && strings.TrimSpace(value) != "" {
			return nil
		}
		break
	}
	return fmt.Errorf("filter term %q has not been verified", term)
}
