package topic

import (
	"reflect"
	"strings"
	"testing"
)

func TestExtractSlugs(t *testing.T) {
	long := "#" + strings.Repeat("a", 101)

	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "basic ASCII",
			content: "A #Hello #world post",
			want:    []string{"hello", "world"},
		},
		{
			name:    "duplicates and case differences",
			content: "#Go #go #GO",
			want:    []string{"go"},
		},
		{
			name:    "Cyrillic",
			content: "#Привет #Мир",
			want:    []string{"привет", "мир"},
		},
		{
			name:    "Uzbek and other Unicode letters",
			content: "#Oʻzbek #Český #日本語",
			want:    []string{"oʻzbek", "český", "日本語"},
		},
		{
			name:    "NFC composed and decomposed equivalents",
			content: "#café #cafe\u0301",
			want:    []string{"café"},
		},
		{
			name:    "digits and underscore",
			content: "#go_1 #2026 #a_b2",
			want:    []string{"go_1", "2026", "a_b2"},
		},
		{
			name:    "punctuation boundaries",
			content: "#one,#two! #three-four (#five).",
			want:    []string{"one", "two", "three", "five"},
		},
		{
			name:    "empty hashtags ignored",
			content: "# #! ##",
			want:    nil,
		},
		{
			name:    "over 100 characters skipped",
			content: "#short " + long + " #after",
			want:    []string{"short", "after"},
		},
		{
			name:    "stable first occurrence order",
			content: "#second #first #second #third #FIRST",
			want:    []string{"second", "first", "third"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExtractSlugs(tt.content); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ExtractSlugs(%q) = %#v, want %#v", tt.content, got, tt.want)
			}
		})
	}
}

func TestCanonicalSlugMatchesExtractionRules(t *testing.T) {
	slug, ok := CanonicalSlug("CafE\u0301_42")
	if !ok || slug != "caf\u00e9_42" {
		t.Fatalf("unexpected canonical slug: %q, %v", slug, ok)
	}
	if _, ok := CanonicalSlug("not-a-topic"); ok {
		t.Fatal("punctuation must not be accepted in a canonical topic slug")
	}
}
