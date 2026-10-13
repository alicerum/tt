package dsl

import (
	"errors"
	"strings"
	"unicode"
)

var (
	ErrEmptyName = errors.New("a name can't be empty")
	ErrNameWords = errors.New("a name is one word, like @ana")
)

// ParseRequester normalizes a typed name the way the @who token does (D53):
// trimmed, one leading @ and any trailing punctuation dropped, lowercased.
// What is left must be one non-empty word.
func ParseRequester(s string) (string, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "@")
	if strings.ContainsFunc(s, unicode.IsSpace) {
		return "", ErrNameWords
	}
	s = strings.ToLower(strings.TrimRight(s, trailingPunct))
	if s == "" {
		return "", ErrEmptyName
	}
	return s, nil
}
