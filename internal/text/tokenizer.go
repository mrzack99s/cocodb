package text

import (
	"strings"
	"unicode"
)

func isAlphaNumASCII(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}

func toLowerASCII(r rune) rune {
	if r >= 'A' && r <= 'Z' {
		return r + ('a' - 'A')
	}
	return r
}

// Tokenize splits text into lowercase alphanumeric tokens with zero allocation on lowercase ASCII substrings.
func Tokenize(text string) []string {
	var tokens []string
	start := -1
	hasUpper := false

	i := 0
	n := len(text)
	for i < n {
		b := text[i]
		if b < 128 {
			if (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') {
				if start < 0 {
					start = i
					hasUpper = false
				}
				i++
			} else if b >= 'A' && b <= 'Z' {
				if start < 0 {
					start = i
				}
				hasUpper = true
				i++
			} else {
				if start >= 0 {
					if !hasUpper {
						tokens = append(tokens, text[start:i])
					} else {
						tokens = append(tokens, strings.ToLower(text[start:i]))
					}
					start = -1
				}
				i++
			}
		} else {
			// Non-ASCII character handling
			r, size := utf8DecodeRune(text[i:])
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				if start < 0 {
					start = i
				}
				hasUpper = true
			} else {
				if start >= 0 {
					tokens = append(tokens, strings.ToLower(text[start:i]))
					start = -1
				}
			}
			i += size
		}
	}
	if start >= 0 {
		if !hasUpper {
			tokens = append(tokens, text[start:n])
		} else {
			tokens = append(tokens, strings.ToLower(text[start:n]))
		}
	}
	return tokens
}

func utf8DecodeRune(s string) (rune, int) {
	for i, r := range s {
		if i > 0 {
			return rune(s[0]), 1
		}
		return r, len(string(r))
	}
	return 0, 1
}
