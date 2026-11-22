package db

import (
	"strings"
	"unicode"
)

var folds = map[rune]string{
	'à': "a", 'â': "a", 'ä': "a", 'á': "a", 'ã': "a", 'å': "a",
	'ç': "c", 'é': "e", 'è': "e", 'ê': "e", 'ë': "e",
	'î': "i", 'ï': "i", 'í': "i", 'ì': "i",
	'ô': "o", 'ö': "o", 'ó': "o", 'ò': "o", 'õ': "o", 'ø': "o",
	'ù': "u", 'û': "u", 'ü': "u", 'ú': "u",
	'ÿ': "y", 'ý': "y", 'ñ': "n", 'œ': "oe", 'æ': "ae", 'ß': "ss",
}

// NameKey lowercases a name, folds accents and keeps only letters and digits
// separated by single spaces, so "Jean-Luc  Lagarce" and "jean luc lagarcé" match.
func NameKey(name string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(name) {
		if f, ok := folds[r]; ok {
			b.WriteString(f)
			space = false
			continue
		}
		switch {
		case r == '\'' || r == '’':
			// "d'Arc" and "dArc" are the same key
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		case b.Len() > 0 && !space:
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.TrimSpace(b.String())
}
