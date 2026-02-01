package runtime

import (
	"regexp"
	"strings"
)

var (
	hyphenStartOrEndRe = regexp.MustCompile(`(^|[^ ])-([^ ]|$)`)
	hyphenMiddleRe     = regexp.MustCompile(`([^ ])-([^ ])`)
)

func FormatKey(key string) string {
	key = strings.ReplaceAll(key, ".", "_")
	key = hyphenStartOrEndRe.ReplaceAllString(key, "${1}_${2}")
	key = hyphenMiddleRe.ReplaceAllString(key, "${1}_${2}")
	return key
}

func FormatExpression(e string) string {
	result := []rune(e)
	openParentheses := 0

	for i, r := range result {
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
