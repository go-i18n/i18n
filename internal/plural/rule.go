// Copyright 2014 Nick Snyder. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package plural

import (
	"golang.org/x/text/language"
)

// Rule defines the CLDR plural rules for a language, see
// http://www.unicode.org/cldr/charts/latest/supplemental/language_plural_rules.html.
type Rule struct {
	PluralForms    map[Form]struct{}
	PluralFormFunc func(*Operands) Form
}

func addPluralRules(rules Rules, ids []string, ps *Rule) {
	for _, id := range ids {
		if id == "root" {
			continue
		}
		tag := language.MustParse(id)
		rules[tag] = ps
	}
}

func newPluralFormSet(pluralForms ...Form) map[Form]struct{} {
	set := make(map[Form]struct{}, len(pluralForms))
	for _, plural := range pluralForms {
		set[plural] = struct{}{}
	}
	return set
}

func intInRange(i, from, to int64) bool {
	return from <= i && i <= to
}

func intEqualsAny(i int64, any ...int64) bool {
	for _, a := range any {
		if i == a {
			return true
		}
	}
	return false
}
