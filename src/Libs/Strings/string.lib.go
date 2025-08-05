package StringLibs

import (
	"regexp"
	"strings"
)

// ConvertToSnakeCase converts CamelCase to snake_case.
func ConvertToSnakeCase(str string) string {
	// Handle acronym edge cases and word boundaries
	snake := regexp.MustCompile("(.)([A-Z][a-z]+)").ReplaceAllString(str, "${1}_${2}")
	snake = regexp.MustCompile("([a-z0-9])([A-Z])").ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}

func Hyphenate(str string) string {
	hyphen := regexp.MustCompile("(.)([A-Z][a-z]+)").ReplaceAllString(str, "${1}-${2}")
	hyphen = regexp.MustCompile("([a-z0-9])([A-Z])").ReplaceAllString(hyphen, "${1}-${2}")
	hyphen = strings.ReplaceAll(hyphen, " ", "-")
	return strings.ToLower(hyphen)
}

// Pluralize adds a basic plural form to a string.
func Pluralize(word string) string {
	// Simple rules
	if strings.HasSuffix(word, "s") || strings.HasSuffix(word, "x") || strings.HasSuffix(word, "z") ||
		strings.HasSuffix(word, "ch") || strings.HasSuffix(word, "sh") {
		return word + "es"
	}
	if strings.HasSuffix(word, "y") && len(word) > 1 && !isVowel(rune(word[len(word)-2])) {
		return word[:len(word)-1] + "ies"
	}
	return word + "s"
}

// isVowel checks if a rune is a vowel.
func isVowel(r rune) bool {
	vowels := "aeiouAEIOU"
	return strings.ContainsRune(vowels, r)
}
