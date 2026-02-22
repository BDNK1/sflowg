package yaml

import (
	"regexp"
	"strings"
)

var (
	hyphenStartOrEndRe = regexp.MustCompile(`(^|[^ ])-([^ ]|$)`)
	hyphenMiddleRe     = regexp.MustCompile(`([^ ])-([^ ])`)
)

func isDigit(r rune) bool {
	return r >= '0' && r <= '9'
}

// FormatKey converts dot-separated and hyphenated keys to underscore-separated flat keys.
// Example: "step.result.field" → "step_result_field"
func FormatKey(key string) string {
	key = strings.ReplaceAll(key, ".", "_")
	key = hyphenStartOrEndRe.ReplaceAllString(key, "${1}_${2}")
	key = hyphenMiddleRe.ReplaceAllString(key, "${1}_${2}")
	return key
}

// FormatExpression converts dot notation in expressions to underscore notation,
// preserving string literals, optional chaining, numeric decimals, and parenthesized expressions.
func FormatExpression(e string) string {
	result := []rune(e)
	openParentheses := 0
	inDoubleQuote := false
	inBacktick := false
	escapeNext := false

	for i, r := range result {
		if escapeNext {
			escapeNext = false
			continue
		}

		if inDoubleQuote && r == '\\' {
			escapeNext = true
			continue
		}

		if r == '"' && !inBacktick {
			inDoubleQuote = !inDoubleQuote
			continue
		}
		if r == '`' && !inDoubleQuote {
			inBacktick = !inBacktick
			continue
		}

		// Don't modify anything inside string literals
		if inDoubleQuote || inBacktick {
			continue
		}

		switch r {
		case '(':
			openParentheses++
		case ')':
			openParentheses--
		case '.':
			// Don't replace dot if it's part of:
			// - ?. (optional chaining operator)
			// - #. (lambda element accessor in expr-lang, e.g., {#.Age > 18})
			if i > 0 && (result[i-1] == '?' || result[i-1] == '#') {
				continue
			}
			// Don't replace dot in numeric literals (e.g., 3.14, 0.5)
			if i > 0 && i < len(result)-1 && isDigit(result[i-1]) && isDigit(result[i+1]) {
				continue
			}
			result[i] = '_'
		case '-':
			if openParentheses == 0 {
				temp := string(result[i-1 : i+2])
				if i == 0 {
					temp = string(result[i : i+2])
				} else if i == len(result)-1 {
					temp = string(result[i-1 : i+1])
				}

				if hyphenStartOrEndRe.MatchString(temp) || hyphenMiddleRe.MatchString(temp) {
					result[i] = '_'
				}

			}
		}
	}
	return string(result)
}
