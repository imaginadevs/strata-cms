package autogen

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var reNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func init() {
	Register(FunctionDef{
		ID:    "slugify",
		Label: "Slugify",
		Description: "Converts a text field to a URL-safe slug: " +
			"unicode-normalized, lowercase, non-alphanumeric characters " +
			"replaced with hyphens. Ideal for the slug field of pages or posts.",
		NeedsSource: true,
	}, slugifyFn)
}

func slugifyFn(ctx Context) (any, error) {
	if ctx.SourceField == "" {
		return nil, nil
	}
	raw := ctx.Item[ctx.SourceField]
	if raw == nil {
		return nil, nil
	}
	return makeSlug(fmt.Sprintf("%v", raw)), nil
}

// makeSlug converts an arbitrary string to a URL-safe slug.
//
// Steps:
//  1. Unicode NFD decomposition so é → e + combining acute
//  2. Strip combining (Mn) characters (accents etc.)
//  3. Recompose to NFC for clean output
//  4. Lowercase
//  5. Collapse runs of non-alphanumeric characters to a single hyphen
//  6. Trim leading/trailing hyphens
func makeSlug(s string) string {
	t := transform.Chain(
		norm.NFD,
		runes.Remove(runes.In(unicode.Mn)),
		norm.NFC,
	)
	normalized, _, err := transform.String(t, s)
	if err != nil {
		normalized = s
	}
	lower := strings.ToLower(normalized)
	slug := reNonAlnum.ReplaceAllString(lower, "-")
	return strings.Trim(slug, "-")
}

// Adding a new built-in is one function + one Register call:
//
//	func init() {
//	    Register(FunctionDef{
//	        ID: "uppercase", Label: "Uppercase",
//	        Description: "...", NeedsSource: true,
//	    }, func(ctx Context) (any, error) {
//	        if ctx.SourceField == "" { return nil, nil }
//	        raw := ctx.Item[ctx.SourceField]
//	        if raw == nil { return nil, nil }
//	        return strings.ToUpper(fmt.Sprintf("%v", raw)), nil
//	    })
//	}
