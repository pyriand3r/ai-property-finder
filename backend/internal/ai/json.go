package ai

import (
	"fmt"
	"propertyfinder/internal/domain"
	"strings"
)

// stripFences removes a leading/trailing Markdown code fence (```json … ```)
// that models frequently wrap JSON in. It only strips a fence that opens the
// trimmed text; prose around a fenced block is handled by extractFirstJSON.
func stripFences(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "```") {
		return trimmed
	}
	if idx := strings.IndexByte(trimmed, '\n'); idx >= 0 {
		trimmed = trimmed[idx+1:]
	} else {
		trimmed = strings.TrimPrefix(trimmed, "```")
	}
	if idx := strings.LastIndex(trimmed, "```"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return strings.TrimSpace(trimmed)
}

// extractFirstJSON returns the first balanced JSON value (object or array)
// in s, tolerating leading/trailing prose and Markdown fences. Braces and
// brackets inside string literals (and escaped quotes) are ignored. Returns
// ErrParse when no balanced value is found.
func extractFirstJSON(s string) (string, error) {
	s = stripFences(s)

	start := strings.IndexAny(s, "{[")
	if start < 0 {
		return "", fmt.Errorf("no JSON value found: %w", ErrParse)
	}

	open := s[start]
	closeByte := byte('}')
	if open == '[' {
		closeByte = ']'
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case open:
			depth++
		case closeByte:
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("unbalanced JSON value: %w", ErrParse)
}

// NormalizeGrade trims and upper-cases a raw grade letter and validates it
// against A-F. Language and surrounding whitespace never gate acceptance —
// only the letter itself does.
func NormalizeGrade(raw string) (domain.CriterionGrade, error) {
	grade := domain.CriterionGrade(strings.ToUpper(strings.TrimSpace(raw)))
	switch grade {
	case domain.GradeA, domain.GradeB, domain.GradeC, domain.GradeD, domain.GradeE, domain.GradeF:
		return grade, nil
	default:
		return "", fmt.Errorf("invalid grade %q: %w", raw, ErrParse)
	}
}
