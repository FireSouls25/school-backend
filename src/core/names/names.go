// Package names folds personal and catalog names for comparison:
// display keeps the original spelling, matching ignores case,
// accents and extra spaces. The ñ stays ñ: it is a distinct letter,
// never folded into n.
package names

import "strings"

var accentFold = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u",
	"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U", "Ü", "U",
)

// Fold canonicalizes s for comparison: trims, collapses inner
// whitespace, lowercases and strips Spanish accents. Display values
// are never replaced by the fold; it only decides equality.
func Fold(s string) string {
	return accentFold.Replace(strings.ToLower(strings.Join(strings.Fields(s), " ")))
}
