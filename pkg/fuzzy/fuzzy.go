// Package fuzzy provides fuzzy matching utilities for Kubernetes resource names.
package fuzzy

import (
	"sort"
	"strings"
	"unicode"
)

// Match represents a single fuzzy match result.
type Match struct {
	// Index is the original index in the input slice.
	Index int
	// Score is the match quality (higher is better).
	Score int
	// Str is the matched string.
	Str string
}

// Find performs fuzzy matching of pattern against items.
// Returns matches sorted by score (best first).
func Find(pattern string, items []string) []Match {
	if pattern == "" {
		matches := make([]Match, len(items))
		for i, s := range items {
			matches[i] = Match{Index: i, Score: 0, Str: s}
		}
		return matches
	}

	pattern = strings.ToLower(pattern)
	var matches []Match

	for i, item := range items {
		score := score(pattern, strings.ToLower(item))
		if score > 0 {
			matches = append(matches, Match{
				Index: i,
				Score: score,
				Str:   item,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	return matches
}

// score computes the fuzzy match score between pattern and text.
// Returns 0 if no match.
func score(pattern, text string) int {
	if len(pattern) == 0 {
		return 1
	}
	if len(text) == 0 {
		return 0
	}

	// Check if all pattern characters appear in order in text.
	pIdx := 0
	totalScore := 0
	prevMatch := -1

	for tIdx := 0; tIdx < len(text) && pIdx < len(pattern); tIdx++ {
		if text[tIdx] == pattern[pIdx] {
			s := 1

			// Bonus for consecutive matches.
			if prevMatch == tIdx-1 {
				s += 4
			}

			// Bonus for matching at word boundary.
			if tIdx == 0 || isBoundary(rune(text[tIdx-1])) {
				s += 3
			}

			// Bonus for matching after separator.
			if tIdx > 0 && (text[tIdx-1] == '-' || text[tIdx-1] == '_' || text[tIdx-1] == '/') {
				s += 2
			}

			// Bonus for exact prefix match.
			if tIdx == pIdx {
				s += 2
			}

			totalScore += s
			prevMatch = tIdx
			pIdx++
		}
	}

	if pIdx < len(pattern) {
		return 0 // Not all pattern characters matched.
	}

	// Bonus for shorter strings (more specific match).
	totalScore += max(0, 20-len(text))

	return totalScore
}

func isBoundary(r rune) bool {
	return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsUpper(r)
}
