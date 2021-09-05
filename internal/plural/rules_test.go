// Copyright 2014 Nick Snyder. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package plural

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/text/language"
)

func TestRules(t *testing.T) {
	expectedRule := &Rule{}

	tests := []struct {
		name     string
		rules    Rules
		tag      language.Tag
		wantRule *Rule
	}{
		{
			name: "exact match",
			rules: Rules{
				language.English: expectedRule,
				language.Spanish: expectedRule,
			},
			tag:      language.English,
			wantRule: expectedRule,
		},
		{
			name: "inexact match",
			rules: Rules{
				language.English: expectedRule,
			},
			tag:      language.AmericanEnglish,
			wantRule: expectedRule,
		},
		{
			name: "portuguese doesn't match european portuguese",
			rules: Rules{
				language.EuropeanPortuguese: expectedRule,
			},
			tag:      language.Portuguese,
			wantRule: nil,
		},
		{
			name: "european portuguese preferred",
			rules: Rules{
				language.Portuguese:         expectedRule,
				language.EuropeanPortuguese: expectedRule,
			},
			tag:      language.EuropeanPortuguese,
			wantRule: expectedRule,
		},
		{
			name: "zh-Hans",
			rules: Rules{
				language.Chinese: expectedRule,
			},
			tag:      language.SimplifiedChinese,
			wantRule: expectedRule,
		},
		{
			name: "zh-Hant",
			rules: Rules{
				language.Chinese: expectedRule,
			},
			tag:      language.TraditionalChinese,
			wantRule: expectedRule,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.rules.Rule(test.tag)
			assert.Equal(t, test.wantRule, got)
		})
	}
}
