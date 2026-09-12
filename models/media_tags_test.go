package models

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Fork (media tags, MEDIA-TAGS-SPEC I1): NormalizeMediaTags trims, lower-folds, dedupes,
// sorts, and rejects anything outside ^[a-z0-9][a-z0-9_-]*$ or longer than 100 chars;
// nil and [] both yield [].
func TestNormalizeMediaTags(t *testing.T) {
	cases := []struct {
		name    string
		in      []string
		want    []string
		wantBad string // the tag named by the error; "" means no error
	}{
		{"nil", nil, []string{}, ""},
		{"empty", []string{}, []string{}, ""},
		{"blank entries dropped", []string{"", "  "}, []string{}, ""},
		{"trim + lower-fold", []string{"  Shala ", "CURATED"}, []string{"curated", "shala"}, ""},
		{"dedupe after folding", []string{"shala", "Shala", " shala"}, []string{"shala"}, ""},
		{"sorted", []string{"social", "loyalty-rewards", "curated"}, []string{"curated", "loyalty-rewards", "social"}, ""},
		{"underscore and hyphen inside", []string{"a_b-c", "0x"}, []string{"0x", "a_b-c"}, ""},
		{"exactly 100 chars", []string{strings.Repeat("a", 100)}, []string{strings.Repeat("a", 100)}, ""},
		{"101 chars rejected", []string{strings.Repeat("a", 101)}, nil, strings.Repeat("a", 101)},
		{"leading hyphen rejected", []string{"-shala"}, nil, "-shala"},
		{"leading underscore rejected", []string{"_shala"}, nil, "_shala"},
		{"brand prefix rejected", []string{"brand:shala"}, nil, "brand:shala"},
		{"inner space rejected", []string{"thirsty girl"}, nil, "thirsty girl"},
		{"non-ascii rejected", []string{"liyorá"}, nil, "liyorá"},
		{"first invalid wins, folded", []string{"ok", "Bad Tag", "-x"}, nil, "bad tag"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := NormalizeMediaTags(c.in)
			if c.wantBad != "" {
				var te *MediaTagInvalidError
				if !errors.As(err, &te) {
					t.Fatalf("want *MediaTagInvalidError, got %v", err)
				}
				if te.Tag != c.wantBad {
					t.Fatalf("error names %q, want %q", te.Tag, c.wantBad)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("got nil slice, want non-nil (serializes [] and binds '{}')")
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v want %v", got, c.want)
			}
		})
	}

	if MediaTagInvalidKey != "media.tagInvalid" {
		t.Fatalf("i18n key drifted: %q", MediaTagInvalidKey)
	}
}
