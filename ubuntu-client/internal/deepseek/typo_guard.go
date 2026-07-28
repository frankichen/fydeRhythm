package deepseek

import "unicode"

// enforceTypoOnly rejects model output that behaves like rewriting instead of
// conservative input-method typo correction. It only permits a small number
// of Han-character substitutions and forbids insertions, deletions, reordering
// or changes to punctuation, spaces, numbers, Latin text and code.
func enforceTypoOnly(original, candidate string) string {
	original = normalizeEnglishPunctuation(original)
	candidate = normalizeEnglishPunctuation(candidate)

	originalRunes := []rune(original)
	candidateRunes := []rune(candidate)
	if len(originalRunes) != len(candidateRunes) {
		return original
	}

	hanCount := 0
	changes := 0
	for i, sourceRune := range originalRunes {
		if unicode.Is(unicode.Han, sourceRune) {
			hanCount++
		}
		if sourceRune == candidateRunes[i] {
			continue
		}
		if !unicode.Is(unicode.Han, sourceRune) ||
			!unicode.Is(unicode.Han, candidateRunes[i]) {
			return original
		}
		changes++
	}

	// Short sentences may contain two independent input mistakes. Longer text
	// allows roughly five percent substitutions, capped to prevent rewriting.
	limit := hanCount / 20
	if limit < 2 {
		limit = 2
	}
	if limit > 8 {
		limit = 8
	}
	if changes > limit {
		return original
	}
	return candidate
}
