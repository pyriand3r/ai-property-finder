package ai

import (
	"errors"
	"propertyfinder/internal/domain"
	"testing"
)

func TestExtractFirstJSON(t *testing.T) {
	cases := map[string]string{
		"plain array":             `[{"a":1}]`,
		"fenced":                  "```json\n[{\"a\":1}]\n```",
		"fenced no lang":          "```\n[{\"a\":1}]\n```",
		"leading prose":           `Here is the result: [{"a":1}]`,
		"trailing prose":          `[{"a":1}] — hope that helps!`,
		"nested braces in string": `[{"a":"contains } and { chars"}]`,
		"escaped quote in string": `[{"a":"she said \"hi\""}]`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := extractFirstJSON(input); err != nil {
				t.Errorf("extractFirstJSON(%q) failed: %v", input, err)
			}
		})
	}
}

func TestExtractFirstJSON_NoValue(t *testing.T) {
	_, err := extractFirstJSON("just prose, no JSON here")
	if !errors.Is(err, ErrParse) {
		t.Errorf("expected ErrParse, got %v", err)
	}
}

func TestExtractFirstJSON_Unbalanced(t *testing.T) {
	_, err := extractFirstJSON(`[{"a":1}`)
	if !errors.Is(err, ErrParse) {
		t.Errorf("expected ErrParse, got %v", err)
	}
}

func TestNormalizeGrade(t *testing.T) {
	cases := []struct {
		input string
		want  domain.CriterionGrade
	}{
		{"A", domain.GradeA},
		{"a", domain.GradeA},
		{" B ", domain.GradeB},
		{"F", domain.GradeF},
	}
	for _, tc := range cases {
		got, err := normalizeGrade(tc.input)
		if err != nil {
			t.Errorf("normalizeGrade(%q) unexpected error: %v", tc.input, err)
		}
		if got != tc.want {
			t.Errorf("normalizeGrade(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}

	if _, err := normalizeGrade("G"); !errors.Is(err, ErrParse) {
		t.Errorf("normalizeGrade(\"G\") should be ErrParse, got %v", err)
	}
	if _, err := normalizeGrade(""); !errors.Is(err, ErrParse) {
		t.Errorf("normalizeGrade(\"\") should be ErrParse, got %v", err)
	}
}
