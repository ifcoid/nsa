package modules

import "testing"

func TestParseCategorization(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  *categorizationBands
	}{
		{
			name:  "standard format with pipes and percent",
			input: "HIGH >=80% | MODERATE 70-79% | LOW <70%",
			want:  &categorizationBands{highMin: 80, moderateMin: 70},
		},
		{
			name:  "compact format no spaces",
			input: "HIGH>=80 MODERATE 70-79 LOW<70",
			want:  &categorizationBands{highMin: 80, moderateMin: 70},
		},
		{
			name:  "different thresholds",
			input: "HIGH >=85% | MODERATE 60-84% | LOW <60%",
			want:  &categorizationBands{highMin: 85, moderateMin: 60},
		},
		{
			name:  "with >= for moderate",
			input: "HIGH >=90% | MODERATE >=75% | LOW <75%",
			want:  &categorizationBands{highMin: 90, moderateMin: 75},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "garbage input",
			input: "some random text",
			want:  nil,
		},
		{
			name:  "only HIGH without MODERATE",
			input: "HIGH >=80",
			want:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseCategorization(tc.input)
			if tc.want == nil {
				if got != nil {
					t.Errorf("parseCategorization(%q) = %+v, want nil", tc.input, got)
				}
				return
			}
			if got == nil {
				t.Fatalf("parseCategorization(%q) = nil, want %+v", tc.input, tc.want)
			}
			if got.highMin != tc.want.highMin {
				t.Errorf("highMin = %v, want %v", got.highMin, tc.want.highMin)
			}
			if got.moderateMin != tc.want.moderateMin {
				t.Errorf("moderateMin = %v, want %v", got.moderateMin, tc.want.moderateMin)
			}
		})
	}
}

func TestCategoryFor(t *testing.T) {
	tests := []struct {
		name           string
		score          float64
		threshold      float64
		categorization string
		want           string
	}{
		// With valid categorization string
		{name: "high with bands", score: 85, threshold: 70, categorization: "HIGH >=80% | MODERATE 70-79% | LOW <70%", want: "HIGH"},
		{name: "moderate with bands", score: 75, threshold: 70, categorization: "HIGH >=80% | MODERATE 70-79% | LOW <70%", want: "MODERATE"},
		{name: "low with bands", score: 60, threshold: 70, categorization: "HIGH >=80% | MODERATE 70-79% | LOW <70%", want: "LOW"},
		{name: "exact high boundary", score: 80, threshold: 70, categorization: "HIGH >=80% | MODERATE 70-79% | LOW <70%", want: "HIGH"},
		{name: "exact moderate boundary", score: 70, threshold: 70, categorization: "HIGH >=80% | MODERATE 70-79% | LOW <70%", want: "MODERATE"},
		{name: "just below moderate", score: 69.9, threshold: 70, categorization: "HIGH >=80% | MODERATE 70-79% | LOW <70%", want: "LOW"},

		// Fallback when categorization is empty
		{name: "fallback high", score: 85, threshold: 70, categorization: "", want: "HIGH"},
		{name: "fallback moderate", score: 75, threshold: 70, categorization: "", want: "MODERATE"},
		{name: "fallback low", score: 60, threshold: 70, categorization: "", want: "LOW"},

		// Fallback when categorization is invalid
		{name: "invalid categorization falls back", score: 85, threshold: 70, categorization: "garbage", want: "HIGH"},

		// No categorization arg at all (variadic empty)
		{name: "no variadic arg high", score: 81, threshold: 70, want: "HIGH"},
		{name: "no variadic arg moderate", score: 75, threshold: 70, want: "MODERATE"},
		{name: "no variadic arg low", score: 60, threshold: 70, want: "LOW"},

		// Custom bands different from threshold+10 default
		{name: "custom bands override", score: 82, threshold: 70, categorization: "HIGH >=85% | MODERATE 60-84% | LOW <60%", want: "MODERATE"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			if tc.categorization != "" || tc.name == "fallback when categorization is empty" || tc.name == "invalid categorization falls back" {
				got = categoryFor(tc.score, tc.threshold, tc.categorization)
			} else {
				got = categoryFor(tc.score, tc.threshold)
			}
			if got != tc.want {
				t.Errorf("categoryFor(%v, %v, %q) = %q, want %q", tc.score, tc.threshold, tc.categorization, got, tc.want)
			}
		})
	}
}
