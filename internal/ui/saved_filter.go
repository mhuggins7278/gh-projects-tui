package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/mhuggins7278/gh-projects-tui/internal/github"
)

// validateSavedFilter gates saved views on query shapes verified against
// ProjectV2.items(query:). GitHub evaluates the original filter; this code
// never rewrites it or tries to evaluate item membership locally.
func validateSavedFilter(filter string) error {
	return validateSavedFilterWithFields(filter, nil)
}

func validateSavedFilterWithFields(filter string, fields []github.Field) error {
	terms, err := splitSavedFilterTerms(filter)
	if err != nil {
		return err
	}
	for _, term := range terms {
		if err := validateSavedFilterTerm(term, fields); err != nil {
			return err
		}
	}
	return nil
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

func validateSavedFilterTerm(term string, fields []github.Field) error {
	negative := strings.HasPrefix(term, "-")
	if negative {
		term = term[1:]
	}
	qualifier, value, found := strings.Cut(term, ":")
	if !found {
		if !negative && !strings.EqualFold(term, "and") && !strings.EqualFold(term, "or") && simpleFilterValue(term, false) {
			return nil // General search is evaluated by GitHub across titles/text fields.
		}
		return unverifiedFilterTerm(term, "use a verified qualifier or an unquoted text search word; Boolean operators are unsupported")
	}
	if qualifier == "" || value == "" {
		return fmt.Errorf("filter term %q needs a qualifier and value (for example, status:\"Todo\")", term)
	}
	values, err := splitFilterValues(value)
	if err != nil {
		return fmt.Errorf("filter term %q: %w", term, err)
	}
	switch qualifier {
	case "status":
		for _, part := range values {
			if !simpleFilterValue(part, true) {
				return unverifiedFilterTerm(term, "Status values must be nonempty words or double-quoted names without escapes")
			}
		}
	case "assignee":
		for _, part := range values {
			if part != "@me" && !identifier(part, false) {
				return unverifiedFilterTerm(term, "assignees must be @me or unquoted usernames")
			}
		}
	case "reviewers":
		for _, part := range values {
			if !identifier(part, false) {
				return unverifiedFilterTerm(term, "reviewers accept unquoted usernames; @me and team references still need matching live probes")
			}
		}
	case "label":
		for _, part := range values {
			if !simpleFilterValue(part, true) && !validWildcardFilter(part) {
				return unverifiedFilterTerm(term, "labels accept nonempty words, double-quoted names without escapes, or a word with leading/trailing * wildcards")
			}
		}
	case "title":
		if len(values) != 1 || !validTitleFilter(values[0]) {
			return unverifiedFilterTerm(term, "Title accepts one exact word, a double-quoted title without escapes, or a word with leading/trailing * wildcards")
		}
	case "repo":
		if len(values) != 1 {
			return unverifiedFilterTerm(term, "multiple repositories have not been verified")
		}
		owner, repo, ok := strings.Cut(values[0], "/")
		if !ok || !identifier(owner, false) || !identifier(repo, true) {
			return unverifiedFilterTerm(term, "use repo:OWNER/REPO")
		}
	case "is":
		if len(values) != 1 || !oneOf(values[0], "open", "closed", "issue", "pr", "draft", "merged") {
			return unverifiedFilterTerm(term, "use one of open, closed, issue, pr, draft, merged")
		}
	case "reason":
		for _, part := range values {
			if !oneOf(part, "completed", `"completed"`, `"not planned"`) {
				return unverifiedFilterTerm(term, `close reasons accept completed or "not planned"; reopened and other reasons still need matching live probes`)
			}
		}
	case "type":
		if len(values) != 1 || !simpleFilterValue(values[0], true) {
			return unverifiedFilterTerm(term, "issue types accept one unquoted word or double-quoted name")
		}
	case "parent-issue":
		if len(values) != 1 || !validParentIssueFilter(values[0]) {
			return unverifiedFilterTerm(term, `use one OWNER/REPO#NUMBER reference, optionally double-quoted; multiple references remain unverified`)
		}
	case "has", "no":
		if negative && qualifier == "has" {
			return unverifiedFilterTerm(term, "use no:FIELD instead of -has:FIELD")
		}
		if len(values) != 1 {
			return unverifiedFilterTerm(term, "presence checks take one field name")
		}
		if !oneOf(values[0], "status", "assignee", "label", "reviewers", "parent-issue", "closed") {
			field, ok := filterField(values[0], fields)
			if !ok || !oneOf(field.DataType, "NUMBER", "ITERATION", "SINGLE_SELECT") {
				return unverifiedFilterTerm(term, "presence checks accept Status, assignee, label, reviewers, parent-issue, closed, or verified number/iteration/single-select project fields")
			}
		}
	case "iteration":
		if negative {
			return unverifiedFilterTerm(term, "negated iteration filters have not been verified; only positive iteration:@current is supported on this qualifier")
		}
		if len(values) != 1 || values[0] != "@current" {
			return unverifiedFilterTerm(term, "only iteration:@current has been probed; other relative iterations need matching live results")
		}
	case "created", "updated", "closed":
		if len(values) != 1 || !validDateFilter(values[0]) {
			return unverifiedFilterTerm(term, "dates accept YYYY-MM-DD, @today offsets, comparisons, or inclusive ranges")
		}
	default:
		field, ok := filterField(qualifier, fields)
		if !ok {
			return unverifiedFilterTerm(term, "this qualifier does not match a verified project field (use its lower-case hyphenated name)")
		}
		switch field.DataType {
		case "SINGLE_SELECT":
			for _, part := range values {
				if !simpleFilterValue(part, true) {
					return unverifiedFilterTerm(term, "single-select values must be nonempty words or double-quoted names without escapes")
				}
			}
		case "NUMBER":
			for _, part := range values {
				if !validNumberFilter(part) {
					return unverifiedFilterTerm(term, "number fields accept values, comparisons, or inclusive ranges such as 1..3 and *..10")
				}
			}
		case "ITERATION":
			if negative {
				return unverifiedFilterTerm(term, "negated iteration filters have not been verified; positive title/relative filters and has:/no: checks are supported")
			}
			if len(values) != 1 || !validIterationFilter(values[0]) {
				return unverifiedFilterTerm(term, "iteration fields accept a quoted title, @current, @previous, comparisons, or ranges between those two keywords")
			}
		default:
			return unverifiedFilterTerm(term, "this field type has not been verified with items(query:); choose a saved view using supported single-select, number, or iteration filters")
		}
	}
	return nil
}

func validTitleFilter(value string) bool {
	if simpleFilterValue(value, true) {
		return true
	}
	// Commas in exact quoted titles were verified separately from commas in
	// label/single-select names. Escaped quotes remain gated on every field.
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		inner := value[1 : len(value)-1]
		return strings.TrimSpace(inner) != "" && !strings.ContainsAny(inner, "\\\"")
	}
	return validWildcardFilter(value)
}

func validWildcardFilter(value string) bool {
	if strings.HasPrefix(value, "**") || strings.HasSuffix(value, "**") {
		return false
	}
	word := strings.Trim(value, "*")
	return word != value && word != "" && strings.Count(value, "*") <= 2 && simpleFilterValue(word, false)
}

// Project field names with spaces use hyphenated qualifiers in items(query:).
// Only the observed ASCII spelling is recognized; collisions stay unsupported.
func filterField(qualifier string, fields []github.Field) (github.Field, bool) {
	var match github.Field
	for _, field := range fields {
		if filterFieldName(field.Name) != qualifier {
			continue
		}
		if match.Name != "" {
			return github.Field{}, false
		}
		match = field
	}
	return match, match.Name != ""
}

func filterFieldName(name string) string {
	for _, char := range name {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == ' ' || char == '-') {
			return ""
		}
	}
	return strings.ToLower(strings.Join(strings.Fields(name), "-"))
}

func validNumberFilter(value string) bool {
	if left, right, ok := strings.Cut(value, ".."); ok {
		return (left == "*" || validNumber(left)) && (right == "*" || validNumber(right)) && !(left == "*" && right == "*")
	}
	for _, op := range []string{">=", "<=", ">", "<"} {
		if strings.HasPrefix(value, op) {
			return validNumber(strings.TrimPrefix(value, op))
		}
	}
	return validNumber(value)
}

func validNumber(value string) bool {
	if strings.HasPrefix(value, "-") {
		value = value[1:]
	}
	if value == "" || value[0] < '0' || value[0] > '9' || value[len(value)-1] < '0' || value[len(value)-1] > '9' || strings.Count(value, ".") > 1 {
		return false
	}
	for _, char := range value {
		if (char >= '0' && char <= '9') || char == '.' {
			continue
		}
		return false
	}
	_, err := strconv.ParseFloat(value, 64)
	return err == nil
}

func validIterationFilter(value string) bool {
	if left, right, ok := strings.Cut(value, ".."); ok {
		return oneOf(left, "@previous", "@current") && oneOf(right, "@previous", "@current")
	}
	for _, op := range []string{">=", "<=", ">", "<"} {
		if strings.HasPrefix(value, op) {
			return oneOf(strings.TrimPrefix(value, op), "@previous", "@current")
		}
	}
	return oneOf(value, "@previous", "@current") || (len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' && simpleFilterValue(value, true))
}

func validParentIssueFilter(value string) bool {
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	ownerRepo, number, ok := strings.Cut(value, "#")
	if !ok || number == "" || strings.Contains(number, "#") {
		return false
	}
	owner, repo, ok := strings.Cut(ownerRepo, "/")
	if !ok || !identifier(owner, false) || !identifier(repo, true) {
		return false
	}
	for _, digit := range number {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	issueNumber, err := strconv.ParseUint(number, 10, 64)
	return err == nil && issueNumber > 0
}

func validDateFilter(value string) bool {
	if left, right, ok := strings.Cut(value, ".."); ok {
		return (left == "*" || validDate(left)) && (right == "*" || validDate(right)) && !(left == "*" && right == "*")
	}
	for _, op := range []string{">=", "<=", ">", "<"} {
		if strings.HasPrefix(value, op) {
			return validDate(strings.TrimPrefix(value, op))
		}
	}
	return validDate(value)
}

func validDate(value string) bool {
	if value == "@today" {
		return true
	}
	if strings.HasPrefix(value, "@today+") || strings.HasPrefix(value, "@today-") {
		offset := value[len("@today+"):]
		if strings.HasSuffix(offset, "d") || strings.HasSuffix(offset, "w") {
			offset = offset[:len(offset)-1]
		}
		if offset == "" {
			return false
		}
		for _, digit := range offset {
			if digit < '0' || digit > '9' {
				return false
			}
		}
		return true
	}
	if len(value) != len("2006-01-02") {
		return false
	}
	date, err := time.Parse("2006-01-02", value)
	return err == nil && date.Format("2006-01-02") == value
}

// Commas inside quoted values are literal; empty components and whitespace
// outside quotes are not a safe way to express multiple values.
func splitFilterValues(value string) ([]string, error) {
	parts := make([]string, 0, 2)
	start, quoted, escaped := 0, false, false
	for index, char := range value {
		if quoted {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				quoted = false
			}
		} else if char == '"' {
			quoted = true
		} else if char == ',' {
			if index == start {
				return nil, fmt.Errorf("comma-separated values cannot be empty")
			}
			parts = append(parts, value[start:index])
			start = index + 1
		}
	}
	if quoted {
		return nil, fmt.Errorf("unterminated quoted filter value")
	}
	if start == len(value) {
		return nil, fmt.Errorf("comma-separated values cannot be empty")
	}
	return append(parts, value[start:]), nil
}

func simpleFilterValue(value string, quoted bool) bool {
	if quoted && len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
		return strings.TrimSpace(value) != "" && !strings.ContainsAny(value, "\\\",")
	}
	if value == "" || strings.ContainsAny(value, "\"'\\,:*<>@") || strings.Contains(value, "..") {
		return false
	}
	for _, char := range value {
		if unicode.IsSpace(char) {
			return false
		}
	}
	return true
}

func identifier(value string, dot bool) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || (dot && (char == '.' || char == '_')) {
			continue
		}
		return false
	}
	return true
}

func oneOf(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}

func unverifiedFilterTerm(term, hint string) error {
	return fmt.Errorf("filter term %q has not been verified: %s", term, hint)
}
