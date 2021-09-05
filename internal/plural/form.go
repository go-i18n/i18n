// Copyright 2014 Nick Snyder. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package plural

// Form represents a language pluralization form as defined here:
// http://cldr.unicode.org/index/cldr-spec/plural-rules
type Form string

// All defined plural forms.
const (
	Zero  Form = "zero"
	One   Form = "one"
	Two   Form = "two"
	Few   Form = "few"
	Many  Form = "many"
	Other Form = "other"
)
