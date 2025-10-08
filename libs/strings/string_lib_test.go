package strlib_test

import (
	"testing"

	strlib "github.com/venomous-maker/go-eloquent/libs/strings"
)

func TestConvertToSnakeCase(t *testing.T) {
	cases := map[string]string{
		"TestModel":      "test_model",
		"HTTPServer":     "http_server",
		"UserID":         "user_id",
		"MyAwesomeThing": "my_awesome_thing",
		"Already_snake":  "already_snake",
		"Camel2HTTP":     "camel2_http",
	}

	for in, want := range cases {
		if got := strlib.ConvertToSnakeCase(in); got != want {
			t.Fatalf("ConvertToSnakeCase(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestHyphenate(t *testing.T) {
	cases := map[string]string{
		"TestModel":      "test-model",
		"MyAwesomeThing": "my-awesome-thing",
		"HTTPServer":     "http-server",
	}

	for in, want := range cases {
		if got := strlib.Hyphenate(in); got != want {
			t.Fatalf("Hyphenate(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestPluralize(t *testing.T) {
	cases := map[string]string{
		"user":     "users",
		"box":      "boxes",
		"category": "categories",
		"church":   "churches",
	}

	for in, want := range cases {
		if got := strlib.Pluralize(in); got != want {
			t.Fatalf("Pluralize(%q) = %q; want %q", in, got, want)
		}
	}
}
