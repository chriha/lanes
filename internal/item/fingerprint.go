package item

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
)

// injected lists fields the UI adds to Fields itself; they are not item data and never change the fingerprint.
var injected = []string{"note", "snoozed"}

// Fingerprint hashes the item's stable data (title, draft, badges and provider fields, without injected and
// pending fields), so a later fetch can tell whether the item changed.
func (i Item) Fingerprint() string {
	fields := make(map[string]any, len(i.Fields))
	for k, v := range i.Fields {
		if !slices.Contains(injected, k) && !slices.Contains(i.Pending, k) {
			fields[k] = v
		}
	}
	// fmt prints map keys sorted and %#v quotes strings, so equal data hashes equal and values can't run together.
	h := sha256.New()
	fmt.Fprintf(h, "%q %t %#v %#v", i.Title, i.Draft, i.Badges, fields)
	return hex.EncodeToString(h.Sum(nil))
}
