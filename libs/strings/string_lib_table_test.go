package strlib_test

import (
	"testing"

	strlib "github.com/venomous-maker/go-eloquent/libs/strings"
)

func TestConvertToSnakeCase_EdgeCases(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"JSONData", "json_data"},
		{"XMLHttpRequest", "xml_http_request"},
		{"äöÜTest", "äö_ü_test"}, // unicode characters
		{"Already_Snake", "already_snake"},
		{"Number123Test", "number123_test"},
	}

	for _, c := range cases {
		got := strlib.ConvertToSnakeCase(c.in)
		if got != c.want {
			t.Fatalf("ConvertToSnakeCase(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestPluralize_EdgeCases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"bus", "buses"},
		{"quiz", "quizzes"},
		{"lady", "ladies"},
		{"boy", "boys"},
		{"sheep", "sheep"}, // naive pluralizer will append s — prefer canonical irregular
	}

	for _, c := range cases {
		if got := strlib.Pluralize(c.in); got != c.want {
			t.Fatalf("Pluralize(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}

func TestHyphenate_EdgeCases(t *testing.T) {
	cases := map[string]string{
		"JSONData": "json-data",
		// Normalize expectation to standard lowercasing used by library
		"İstanbulCity": "istanbul-city",
	}

	for in, want := range cases {
		if got := strlib.Hyphenate(in); got != want {
			t.Fatalf("Hyphenate(%q) = %q; want %q", in, got, want)
		}
	}
}
