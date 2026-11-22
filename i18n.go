// Copyright 2021 Joe Chen. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package i18n

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/text/language"
	"unknwon.dev/ini/v2"

	"unknwon.dev/i18n/internal/plural"
)

// Store contains a collection of locales and their descriptive names.
type Store struct {
	langs   []string
	descs   []string
	locales map[string]*Locale
	rules   plural.Rules
}

// NewStore initializes and returns a new Store.
func NewStore() *Store {
	return &Store{
		locales: make(map[string]*Locale),
		rules:   plural.DefaultRules(),
	}
}

// add attempts to add the given locale into the store. It returns true if it
// was successfully added, false if a locale with the same language name has
// already existed.
func (s *Store) add(l *Locale) bool {
	if _, ok := s.locales[l.Lang()]; ok {
		return false
	}

	s.langs = append(s.langs, l.Lang())
	s.descs = append(s.descs, l.Description())
	s.locales[l.Lang()] = l

	return true
}

// AddLocale adds a locale with given language name and description that is
// loaded from the list of sources. Please refer to INI documentation regarding
// what is considered as a valid data source:
// https://pkg.go.dev/unknwon.dev/ini/v2#LoadSources.
func (s *Store) AddLocale(lang, desc string, source interface{}, others ...interface{}) (*Locale, error) {
	tag, err := language.Parse(lang)
	if err != nil {
		return nil, errors.Wrap(err, "parse lang")
	}

	file, err := ini.LoadSources(
		append([]interface{}{source}, others...),
		ini.IgnoreInlineComment(true),
		ini.UnescapeValueCommentSymbols(true),
		ini.Locking(false), // The file is only read.
	)
	if err != nil {
		return nil, errors.Wrap(err, "load sources")
	}

	rule := s.rules[tag]
	if rule == nil {
		base, confidence := tag.Base()
		if confidence != language.No {
			rule = s.rules[language.MustParse(base.String())]
		}
	}

	l, err := newLocale(tag, desc, rule, file)
	if err != nil {
		return nil, errors.Wrap(err, "new locale")
	}
	if !s.add(l) {
		return nil, errors.Errorf("duplicated locales for %q", lang)
	}
	return l, nil
}

var ErrLocalNotFound = errors.New("locale not found")

// Locale returns the locale with the given language name.
func (s *Store) Locale(lang string) (*Locale, error) {
	l, ok := s.locales[lang]
	if !ok {
		return nil, ErrLocalNotFound
	}
	return l, nil
}

type pluralPlaceholder struct {
	name  string
	arg   string // The name of the argument for a named plural, e.g. "count" in "${file, count}".
	forms map[plural.Form]string
	formsStats
}

// formsStats describes the forms of a plural, computed once per locale.
type formsStats struct {
	longest    int   // The length of the longest form.
	directives int   // The number of fmt directives in all forms.
	formatErr  error // Why the forms are unsafe to format with fmt.Sprintf.
}

// form returns the plural form that matches the given count.
func (p *pluralPlaceholder) form(rule *plural.Rule, count interface{}) (string, error) {
	ops, err := plural.NewOperands(count)
	if err != nil {
		return "", err
	}

	form := plural.Other
	if rule != nil {
		form = rule.PluralFormFunc(ops)
	}
	return p.forms[form], nil
}

// Message represents a message in a locale.
type Message struct {
	pluralRule *plural.Rule
	format     string
	// formatErr is why the message is unsafe to translate with positional
	// arguments, which go through fmt.Sprintf.
	formatErr error
	// positionalParts is the format split for translating with positional
	// arguments, nil when the format has no plural.
	positionalParts []positionalPart
	// namedParts is the format split for translating with named arguments, nil when the
	// format has no placeholder to fill.
	namedParts []namedPart
}

// namedPart is a literal text of a message followed by either a named argument
// or a named plural to fill in.
type namedPart struct {
	text   string
	name   string // e.g. "path" for "{path}"
	plural *pluralPlaceholder
}

// Translate translates the message with the supplied list of arguments.
func (m *Message) Translate(args ...interface{}) string {
	if len(args) == 0 {
		return m.format
	}
	if m.formatErr != nil {
		return "<" + m.formatErr.Error() + ">"
	}
	if m.positionalParts == nil {
		return fmt.Sprintf(m.format, args...)
	}

	var b strings.Builder
	b.Grow(len(m.format))
	for _, p := range m.positionalParts {
		b.WriteString(p.text)
		switch {
		case p.plural == nil:
		case len(args) < p.index:
			b.WriteString("<no arg for index ")
			b.WriteString(strconv.Itoa(p.index))
			b.WriteByte('>')
		default:
			form, err := p.plural.form(m.pluralRule, args[p.index-1])
			if err != nil {
				// The error may quote the argument, which must not add verbs to the format.
				form = strings.ReplaceAll("<"+err.Error()+">", "%", "%%")
			}
			b.WriteString(form)
		}
	}
	return fmt.Sprintf(b.String(), args...)
}

// positionalPart is a literal text of a message followed by an optional plural
// to fill in from the positional argument at index.
type positionalPart struct {
	text   string
	index  int
	plural *pluralPlaceholder
}

// parsePositionalParts splits the format into parts for translating with
// positional arguments, nil when the format has no plural. Only the first
// occurrence of each positional plural is filled, and named plurals are
// reported as missing arguments. It also returns the format with named plurals
// reported as missing arguments.
func parsePositionalParts(format string, placeholders map[int]*pluralPlaceholder, namedPlurals map[string]*pluralPlaceholder) ([]positionalPart, string) {
	if len(placeholders) == 0 && len(namedPlurals) == 0 {
		return nil, format
	}

	// A single pass keeps loading linear in the length of the format.
	var parts []positionalPart
	var text, positional strings.Builder
	filled := make(map[int]bool, len(placeholders))
	rest := format
	for {
		open := strings.Index(rest, "${")
		if open < 0 {
			break
		}
		// Placeholder names have no "$", "{" or "}", so each byte is scanned at
		// most once.
		end := open + 2
		for end < len(rest) && rest[end] != '$' && rest[end] != '{' && rest[end] != '}' {
			end++
		}
		if end == len(rest) || rest[end] != '}' {
			text.WriteString(rest[:open+1])
			positional.WriteString(rest[:open+1])
			rest = rest[open+1:]
			continue
		}

		name := rest[open+2 : end]
		if placeholder, ok := namedPlurals[name]; ok {
			missing := "<no arg for name " + placeholder.arg + ">"
			text.WriteString(rest[:open])
			text.WriteString(missing)
			positional.WriteString(rest[:open])
			positional.WriteString(missing)
			rest = rest[end+1:]
			continue
		}

		index, err := strconv.Atoi(name)
		if placeholder, ok := placeholders[index]; err == nil && ok && !filled[index] && placeholder.name == rest[open:end+1] {
			filled[index] = true
			text.WriteString(rest[:open])
			parts = append(parts, positionalPart{text: text.String(), index: index, plural: placeholder})
			text.Reset()
		} else {
			text.WriteString(rest[:end+1])
		}
		positional.WriteString(rest[:end+1])
		rest = rest[end+1:]
	}
	text.WriteString(rest)
	positional.WriteString(rest)
	return append(parts, positionalPart{text: text.String()}), positional.String()
}

// isPlaceholderNameByte returns true if c is an ASCII letter, digit or
// underscore.
func isPlaceholderNameByte(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '_'
}

// isPlaceholderName returns true if the name only consists of ASCII letters,
// digits and underscores.
func isPlaceholderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		if !isPlaceholderNameByte(name[i]) {
			return false
		}
	}
	return true
}

// parseNamedParts splits the format into parts for translating with named
// arguments, in a single pass so that a value containing a placeholder is never
// substituted again.
func parseNamedParts(format string, placeholders map[int]*pluralPlaceholder, namedPlurals map[string]*pluralPlaceholder) []namedPart {
	var parts []namedPart
	var text strings.Builder
	start := 0 // The start of the format that is not yet written to text.
	next := 0  // Where to look for the next placeholder.
	for {
		n := strings.IndexByte(format[next:], '{')
		if n < 0 {
			break
		}
		open := next + n
		n = strings.IndexByte(format[open+1:], '}')
		if n < 0 {
			break
		}
		end := open + 1 + n
		// Only the last brace before the closing one may open a placeholder,
		// e.g. "{{name}", and skipping the others keeps loading linear.
		open += strings.LastIndexByte(format[open:end], '{')
		name := format[open+1 : end]
		next = end + 1

		if open > 0 && format[open-1] == '$' {
			if placeholder, ok := namedPlurals[name]; ok {
				text.WriteString(format[start : open-1])
				parts = append(parts, namedPart{text: text.String(), plural: placeholder})
				text.Reset()
				start = next
				continue
			}

			index, err := strconv.Atoi(name)
			if placeholder, ok := placeholders[index]; err == nil && ok && placeholder.name == format[open-1:end+1] {
				text.WriteString(format[start : open-1])
				text.WriteString("<no arg for index " + strconv.Itoa(index) + ">")
				start = next
				continue
			}
		}

		if !isPlaceholderName(name) {
			continue
		}

		text.WriteString(format[start:open])
		parts = append(parts, namedPart{text: text.String(), name: name})
		text.Reset()
		start = next
	}
	if parts == nil && text.Len() == 0 {
		return nil
	}
	text.WriteString(format[start:])
	return append(parts, namedPart{text: text.String()})
}

// namedArg returns the value of the named argument in name/value pairs.
func namedArg(pairs []interface{}, name string) (interface{}, bool) {
	// Messages have few placeholders, so a linear scan beats building a map.
	for i := 0; i+1 < len(pairs); i += 2 {
		if n, ok := pairs[i].(string); ok && n == name {
			return pairs[i+1], true
		}
	}
	return nil, false
}

// TranslateNamed translates the message with named arguments given as
// name/value pairs, e.g. TranslateNamed("path", "README.md", "repo", "gogs").
// Each "{name}" placeholder is replaced by the value of the argument with the
// same name, and the message is not formatted by fmt.Sprintf. A placeholder
// without a matching argument is left unchanged.
func (m *Message) TranslateNamed(pairs ...interface{}) string {
	if m.namedParts == nil {
		return m.format
	}

	var b strings.Builder
	b.Grow(len(m.format))
	for _, p := range m.namedParts {
		b.WriteString(p.text)
		switch {
		case p.plural != nil:
			count, ok := namedArg(pairs, p.plural.arg)
			if ok {
				form, err := p.plural.form(m.pluralRule, count)
				if err != nil {
					form = "<" + err.Error() + ">"
				}
				b.WriteString(form)
			} else {
				b.WriteString("<no arg for name ")
				b.WriteString(p.plural.arg)
				b.WriteByte('>')
			}
		case p.name != "":
			value, ok := namedArg(pairs, p.name)
			if !ok {
				b.WriteByte('{')
				b.WriteString(p.name)
				b.WriteByte('}')
				continue
			}
			// Common types skip fmt, which also keeps b on the stack.
			switch v := value.(type) {
			case string:
				b.WriteString(v)
			case int:
				b.WriteString(strconv.Itoa(v))
			case int64:
				b.WriteString(strconv.FormatInt(v, 10))
			default:
				b.WriteString(fmt.Sprint(v)) //nolint:staticcheck // fmt.Fprint(&b, ...) would move b to the heap.
			}
		}
	}
	return b.String()
}

// Locale represents a locale with target language and a collection of messages.
type Locale struct {
	tag      language.Tag
	desc     string
	messages map[string]*Message
}

// e.g. ${file, 1} => ["file", "1"], ${file, count} => ["file", "count"]
var placeholderRe = regexp.MustCompile(`\${([a-zA-z]+),\s*(\d+|[a-zA-Z_][a-zA-Z0-9_]*)}`)

// newLocale creates a new Locale with given language tag, description and the
// raw locale file. The "[plurals]" section is reserved to define all plurals.
func newLocale(tag language.Tag, desc string, rule *plural.Rule, file *ini.File) (*Locale, error) {
	const pluralsSection = "plurals"
	s := file.Section(pluralsSection)
	keys := s.Keys()
	pluralForms := make(map[string]map[plural.Form]string, len(keys))
	for _, k := range s.Keys() {
		fields := strings.SplitN(k.Name(), ".", 2)
		if len(fields) != 2 {
			continue
		}

		noun, form := fields[0], fields[1]

		p, ok := pluralForms[noun]
		if !ok {
			p = make(map[plural.Form]string, 6)
			pluralForms[noun] = p
		}

		switch plural.Form(form) {
		case plural.Zero, plural.One, plural.Two, plural.Few, plural.Many, plural.Other:
			p[plural.Form(form)] = k.String()
		}
	}
	// Scanning the forms once keeps loading linear in the number of messages.
	pluralStats := make(map[string]formsStats, len(pluralForms))
	for noun, forms := range pluralForms {
		var stats formsStats
		for _, form := range forms {
			stats.longest = max(stats.longest, len(form))
			n, open, err := countDirectives(form)
			stats.directives += n
			if err == nil && open {
				err = errors.New("plural form ends within a format directive")
			}
			if stats.formatErr == nil {
				stats.formatErr = err
			}
		}
		pluralStats[noun] = stats
	}

	messages := make(map[string]*Message)
	for _, s := range file.Sections() {
		if s.Name() == pluralsSection {
			continue
		}

		for _, k := range s.Keys() {
			// NOTE: Majority of messages do not need to deal with plurals, thus it makes
			//  sense to leave them with a nil map to save some memory space.
			var placeholders map[int]*pluralPlaceholder
			// namedPlurals is keyed by the content of the placeholder within the format,
			// e.g. "file:count" for "${file:count}" that was parsed from "${file, count}".
			var namedPlurals map[string]*pluralPlaceholder

			format := k.String()
			if strings.Contains(format, "${") {
				matches := placeholderRe.FindAllStringSubmatch(format, -1)
				replaces := make([]string, 0, len(matches)*2)
				placeholders = make(map[int]*pluralPlaceholder, len(matches))
				for _, submatch := range matches {
					placeholder := submatch[0]
					noun := submatch[1]
					forms, ok := pluralForms[noun]

					arg := submatch[2]
					if c := arg[0]; c < '0' || c > '9' {
						if !ok {
							replaces = append(replaces, placeholder, fmt.Sprintf("<no such plural: %s>", noun))
							continue
						}

						key := noun + ":" + arg
						name := "${" + key + "}"
						replaces = append(replaces, placeholder, name)
						if namedPlurals == nil {
							namedPlurals = make(map[string]*pluralPlaceholder)
						}
						namedPlurals[key] = &pluralPlaceholder{
							name:       name,
							arg:        arg,
							forms:      forms,
							formsStats: pluralStats[noun],
						}
						continue
					}

					index, _ := strconv.Atoi(arg) // An index out of range is clamped and reported as missing.
					if index < 1 {
						return nil, errors.Errorf("the smallest index is 1 but got %d for %q", index, placeholder)
					}

					if !ok {
						replaces = append(replaces, placeholder, fmt.Sprintf("<no such plural: %s>", noun))
						continue
					}

					name := fmt.Sprintf("${%d}", index)
					replaces = append(replaces, placeholder, name)
					placeholders[index] = &pluralPlaceholder{
						name:       name,
						forms:      forms,
						formsStats: pluralStats[noun],
					}
				}
				format = strings.NewReplacer(replaces...).Replace(format)
			}

			key := strings.TrimPrefix(s.Name()+"::"+k.Name(), ini.DefaultSection+"::")
			positionalParts, positionalFormat := parsePositionalParts(format, placeholders, namedPlurals)
			namedParts := parseNamedParts(format, placeholders, namedPlurals)
			if err := checkPluralLimits(positionalParts, namedParts); err != nil {
				return nil, errors.Wrapf(err, "message %q", key)
			}
			messages[key] = &Message{
				pluralRule:      rule,
				format:          positionalFormat,
				formatErr:       checkFormat(positionalFormat, positionalParts),
				positionalParts: positionalParts,
				namedParts:      namedParts,
			}
		}
	}

	return &Locale{
		tag:      tag,
		desc:     desc,
		messages: messages,
	}, nil
}

// Limits of a message that stop a crafted locale file from making a single
// translation allocate far more than the message and its arguments.
const (
	// maxPluralsLen is the most bytes of plural forms a message may expand to.
	maxPluralsLen = 1 << 20
	// maxPlaceholders is the most placeholders a message may have, and the most
	// fmt directives it may have to be translated with positional arguments.
	// Each of them may repeat an argument.
	maxPlaceholders = 1000
	// maxDirectiveDigits is the most digits of the width or precision of a fmt
	// directive for the message to be translated with positional arguments.
	maxDirectiveDigits = 3
)

// checkPluralLimits returns an error if translating the message may expand
// beyond maxPluralsLen, or if it has more than maxPlaceholders named
// placeholders.
func checkPluralLimits(positionalParts []positionalPart, namedParts []namedPart) error {
	// Checking on every addition keeps the sums from overflowing.
	pluralsLen := 0
	for _, p := range positionalParts {
		if p.plural == nil {
			continue
		}
		pluralsLen += p.plural.longest
		if pluralsLen > maxPluralsLen {
			return errors.Errorf("plurals may expand to more than %d bytes", maxPluralsLen)
		}
	}

	pluralsLen = 0
	placeholders := 0
	for _, p := range namedParts {
		if p.plural == nil && p.name == "" {
			continue
		}
		placeholders++
		if placeholders > maxPlaceholders {
			return errors.Errorf("more than %d placeholders", maxPlaceholders)
		}
		if p.plural != nil {
			pluralsLen += p.plural.longest
			if pluralsLen > maxPluralsLen {
				return errors.Errorf("plurals may expand to more than %d bytes", maxPluralsLen)
			}
		}
	}
	return nil
}

// checkFormat returns an error if the message is unsafe to format with
// fmt.Sprintf after filling its positional plurals.
func checkFormat(positionalFormat string, positionalParts []positionalPart) error {
	if positionalParts == nil {
		n, _, err := countDirectives(positionalFormat)
		if err == nil && n > maxPlaceholders {
			err = errors.Errorf("more than %d format directives", maxPlaceholders)
		}
		return err
	}

	// Each text and plural form is scanned on its own, so no directive may span
	// from one to the next, as fmt.Sprintf sees them joined.
	directives := 0
	for _, p := range positionalParts {
		n, open, err := countDirectives(p.text)
		if err != nil {
			return err
		}
		directives += n
		if p.plural == nil {
			continue
		}
		if open {
			return errors.New("format directive continues into a plural")
		}
		if p.plural.formatErr != nil {
			return p.plural.formatErr
		}
		directives += p.plural.directives
		if directives > maxPlaceholders {
			break
		}
	}
	if directives > maxPlaceholders {
		return errors.Errorf("more than %d format directives", maxPlaceholders)
	}
	return nil
}

// countDirectives returns the number of fmt directives in the format, and
// whether the format ends within a directive. It returns an error if a
// directive has a width or precision that is too large or given by an argument
// with "*".
func countDirectives(format string) (n int, open bool, err error) {
	// The format has no "]" after lastClose, which keeps skipping argument
	// indexes linear.
	lastClose := strings.LastIndexByte(format, ']')
	skipArgIndex := func(i int) int {
		if i >= len(format) || format[i] != '[' {
			return i
		}
		if i > lastClose {
			open = true // A later text may close it.
			return i + 1
		}
		return i + strings.IndexByte(format[i:], ']') + 1
	}

	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		if i < len(format) && format[i] == '%' {
			continue
		}
		n++

		for i < len(format) && strings.IndexByte("+-# 0", format[i]) >= 0 {
			i++
		}
		for pass := 0; pass < 2; pass++ { // The width, then the precision.
			i = skipArgIndex(i)
			if i < len(format) && format[i] == '*' {
				return 0, false, errors.New("format directive takes its width or precision from an argument")
			}
			digits := 0
			for i < len(format) && '0' <= format[i] && format[i] <= '9' {
				digits++
				i++
			}
			if digits > maxDirectiveDigits {
				return 0, false, errors.Errorf("format directive has a width or precision of more than %d digits", maxDirectiveDigits)
			}
			if pass == 0 && i < len(format) && format[i] == '.' {
				i++
				continue
			}
			break
		}
		i = skipArgIndex(i)
		if i >= len(format) {
			open = true
		}
		// The loop moves past the verb, even if it is "%" as in "%5%".
	}
	return n, open, nil
}

// Lang returns the BCP 47 language name of the locale.
func (l *Locale) Lang() string {
	return l.tag.String()
}

// Description returns the descriptive name of the locale.
func (l *Locale) Description() string {
	return l.desc
}

// Translate uses the locale to translate the message of the given key.
func (l *Locale) Translate(key string, args ...interface{}) string {
	return l.TranslateWithFallback(nil, key, args...)
}

// TranslateWithFallback uses the locale to translate the message of the given
// key. It attempts to use the `fallback` to translate if the given key does not
// exist in the locale.
func (l *Locale) TranslateWithFallback(fallback *Locale, key string, args ...interface{}) string {
	m, ok := l.messages[key]
	if !ok {
		if fallback != nil {
			return fallback.Translate(key, args...)
		}
		return fmt.Sprintf("<no such key: %s>", key)
	}
	return m.Translate(args...)
}

// TranslateNamed uses the locale to translate the message of the given key with
// named arguments given as name/value pairs, see Message.TranslateNamed.
func (l *Locale) TranslateNamed(key string, pairs ...interface{}) string {
	return l.TranslateNamedWithFallback(nil, key, pairs...)
}

// TranslateNamedWithFallback is like TranslateNamed but attempts to use the
// `fallback` to translate if the given key does not exist in the locale.
func (l *Locale) TranslateNamedWithFallback(fallback *Locale, key string, pairs ...interface{}) string {
	m, ok := l.messages[key]
	if !ok {
		if fallback != nil {
			return fallback.TranslateNamed(key, pairs...)
		}
		return fmt.Sprintf("<no such key: %s>", key)
	}
	return m.TranslateNamed(pairs...)
}
