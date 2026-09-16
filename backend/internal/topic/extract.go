// Package topic contains topic-specific domain helpers.
package topic

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

const maxSlugRunes = 100

// ExtractSlugs returns distinct canonical hashtag slugs in first-occurrence
// order. A hashtag begins with # and continues through Unicode letters,
// digits, combining marks, and underscores.
func ExtractSlugs(content string) []string {
	runes := []rune(content)
	var slugs []string
	seen := make(map[string]struct{})

	for i := 0; i < len(runes); i++ {
		if runes[i] != '#' {
			continue
		}

		i++
		start := i
		for i < len(runes) && isHashtagRune(runes[i]) {
			i++
		}
		if start == i {
			i--
			continue
		}

		slug := strings.ToLower(norm.NFC.String(string(runes[start:i])))
		if len([]rune(slug)) <= maxSlugRunes {
			if _, exists := seen[slug]; !exists {
				seen[slug] = struct{}{}
				slugs = append(slugs, slug)
			}
		}

		i--
	}

	return slugs
}

func isHashtagRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r)
}
