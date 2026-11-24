package search

import "strings"

// Result is one scored match.
type Result struct {
	Score     int
	NameIndex int // byte offset of the first matched rune inside the name, -1 when matched in path
}

const (
	scoreNamePrefix  = 100
	scoreNameWord    = 70
	scoreNameSubstr  = 40
	scorePathPrefix  = 30
	scorePathWord    = 20
	scorePathSubstr  = 10
	scoreEmptyQuery  = 1
	scoreExactName   = 60
	scoreTokenRepeat = 2
)

// Normalize lowercases and collapses whitespace so queries and names compare consistently.
func Normalize(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// Query splits q into normalized tokens. An empty result means "no filtering".
func Query(q string) []string {
	normalized := Normalize(q)
	if normalized == "" {
		return nil
	}
	return strings.Split(normalized, " ")
}

// Match scores item name and target path against the query tokens. Every token must match, so
// typing more words narrows the result set instead of adding unrelated hits. A name match always
// beats a path match, and inside a field a prefix beats a word start, which beats a substring.
func Match(tokens []string, name, path string) (Result, bool) {
	if len(tokens) == 0 {
		return Result{Score: scoreEmptyQuery, NameIndex: -1}, true
	}

	lowerName := Normalize(name)
	lowerPath := Normalize(path)

	total := 0
	firstIdx := -1
	for _, token := range tokens {
		score, idx, ok := matchToken(token, lowerName, lowerPath)
		if !ok {
			return Result{}, false
		}
		total += score
		if firstIdx < 0 && idx >= 0 {
			firstIdx = idx
		}
	}

	if lowerName == strings.Join(tokens, " ") {
		total += scoreExactName
	}
	return Result{Score: total, NameIndex: firstIdx}, true
}

func matchToken(token, name, path string) (score int, nameIdx int, ok bool) {
	if idx := strings.Index(name, token); idx >= 0 {
		switch {
		case idx == 0:
			return scoreNamePrefix, idx, true
		case isWordStart(name, idx):
			return scoreNameWord, idx, true
		default:
			return scoreNameSubstr, idx, true
		}
	}
	if idx := strings.Index(path, token); idx >= 0 {
		if isWordStart(path, idx) {
			return scorePathWord, -1, true
		}
		return scorePathSubstr, -1, true
	}
	return 0, -1, false
}

// isWordStart reports whether the match at idx begins a word: the previous byte is a separator
// such as a space, or a lower-to-upper transition inside a CamelCase name.
func isWordStart(s string, idx int) bool {
	if idx <= 0 {
		return true
	}
	prev := s[idx-1]
	if prev == ' ' || prev == '-' || prev == '_' || prev == '.' || prev == '/' || prev == '\\' || prev == '(' {
		return true
	}
	return false
}
