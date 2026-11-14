package security

import (
	"regexp"
	"strings"
	"unicode"
)

var injectionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)ignore\s+(all\s+)?(previous|prior|above)\s+instructions?`),
	regexp.MustCompile(`(?i)disregard\s+(all\s+)?(previous|prior|system)\s+instructions?`),
	regexp.MustCompile(`(?i)<\|?(system|im_start|im_end|assistant|user)\|?>`),
	regexp.MustCompile(`(?i)^\s*(system|developer)\s*:\s*`),
	regexp.MustCompile(`(?i)execute\s+(shell|bash|rm\s+-rf|curl\s+.*\|)`),
}

// SanitizeUntrustedText strips control chars and neutralizes common prompt-injection directives.
func SanitizeUntrustedText(raw string) (string, int) {
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		if r == '\n' || r == '\t' || !unicode.IsControl(r) {
			b.WriteRune(r)
		}
	}
	cleaned := b.String()
	hits := 0
	for _, re := range injectionPatterns {
		if re.MatchString(cleaned) {
			hits++
			cleaned = re.ReplaceAllString(cleaned, "[REDACTED_UNTRUSTED_DIRECTIVE]")
		}
	}
	return cleaned, hits
}
