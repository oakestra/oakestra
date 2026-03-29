package kernel

import (
	"cmp"
	"strconv"
	"strings"
	"unicode"
)

type Version struct {
	tokens []token
}

type tokenType int

const (
	tokenTypeNumber    tokenType = iota
	tokenTypeAlpha     tokenType = iota
	tokenTypeSeparator tokenType = iota
)

type token struct {
	typ    tokenType
	number int    // if typ == tokenTypeNumber
	alpha  string // if typ == tokenTypeAlpha
}

func NewVersion(s string) *Version {
	return &Version{
		tokens: tokenizeVersion(s),
	}
}

// IsNewer returns true if k>o.
func (v *Version) IsNewer(o *Version) bool {
	return v.Compare(o) > 0
}

// Compare returns –1 if k<o, 0 if k=o, +1 if k>o.
func (v *Version) Compare(o *Version) int {
	thisTokens := v.tokens
	otherTokens := o.tokens
	n := min(len(thisTokens), len(otherTokens))

	// compare versions token by token for as many tokens as possible
	for i := 0; i < n; i++ {
		a, b := thisTokens[i], otherTokens[i]
		if a.typ != b.typ {
			// Order: Number < Alpha < Separator
			return cmp.Compare(a.typ, b.typ)
		}

		switch a.typ {
		case tokenTypeNumber:
			if res := cmp.Compare(a.number, b.number); res != 0 {
				return res
			}
		case tokenTypeAlpha:
			if res := strings.Compare(a.alpha, b.alpha); res != 0 {
				return res
			}
		case tokenTypeSeparator:
			// separators all compare equal
			continue
		}
	}

	// if both versions have a shared prefix, the one with fewer tokens is considered older/less
	return cmp.Compare(len(thisTokens), len(otherTokens))
}

// tokenizeVersion splits s into runs of digits, letters (lower-cased), or separators (non-alphanumeric).
func tokenizeVersion(s string) []token {
	var tokens []token
	for i := 0; i < len(s); {
		r := rune(s[i])

		switch {
		case unicode.IsDigit(r):
			var run string
			run, i = readRun(s, i, unicode.IsDigit)
			n, _ := strconv.Atoi(run)
			tokens = append(tokens, token{
				typ:    tokenTypeNumber,
				number: n,
				alpha:  "",
			})
		case unicode.IsLetter(r):
			var run string
			run, i = readRun(s, i, unicode.IsLetter)
			tokens = append(tokens, token{
				typ:    tokenTypeAlpha,
				number: 0,
				alpha:  run,
			})
		default:
			_, i = readRun(s, i, func(r rune) bool {
				return !unicode.IsDigit(r) && !unicode.IsLetter(r)
			})
			tokens = append(tokens, token{
				typ:    tokenTypeSeparator,
				number: 0,
				alpha:  "",
			})
		}
	}
	return tokens
}

// readRun returns the substring of s starting at i as long as cond(rune)
// is true, and the index immediately after that run.
func readRun(s string, i int, cond func(rune) bool) (string, int) {
	j := i
	for j < len(s) && cond(rune(s[j])) {
		j++
	}
	return s[i:j], j
}
