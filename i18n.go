// Copyright 2021 Joe Chen. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package i18n

import (
	"fmt"
	"regexp"
	"sort"
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
}

// form returns the plural form that matches the given count.
func (p *pluralPlaceholder) form(rule *plural.Rule, count interface{}) string {
	ops, err := plural.NewOperands(count)
	if err != nil {
		return fmt.Sprintf("<%v>", err)
	}

	form := plural.Other
	if rule != nil {
		form = rule.PluralFormFunc(ops)
	}
	return p.forms[form]
}

// Message represents a message in a locale.
type Message struct {
	pluralRule *plural.Rule
	format     string
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
			b.WriteString(p.plural.form(m.pluralRule, args[p.index-1]))
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
// reported as missing arguments.
func parsePositionalParts(format string, placeholders map[int]*pluralPlaceholder, namedPlurals map[string]*pluralPlaceholder) []positionalPart {
	if len(placeholders) == 0 && len(namedPlurals) == 0 {
		return nil
	}

	for _, placeholder := range namedPlurals {
		format = strings.ReplaceAll(format, placeholder.name, "<no arg for name "+placeholder.arg+">")
	}

	type occurrence struct {
		pos   int
		index int
	}
	occurrences := make([]occurrence, 0, len(placeholders))
	for index, placeholder := range placeholders {
		if pos := strings.Index(format, placeholder.name); pos >= 0 {
			occurrences = append(occurrences, occurrence{pos: pos, index: index})
		}
	}
	sort.Slice(occurrences, func(i, j int) bool { return occurrences[i].pos < occurrences[j].pos })

	parts := make([]positionalPart, 0, len(occurrences)+1)
	start := 0
	for _, o := range occurrences {
		placeholder := placeholders[o.index]
		parts = append(parts, positionalPart{
			text:   format[start:o.pos],
			index:  o.index,
			plural: placeholder,
		})
		start = o.pos + len(placeholder.name)
	}
	return append(parts, positionalPart{text: format[start:]})
}

// isPlaceholderName returns true if the name only consists of ASCII letters,
// digits and underscores.
func isPlaceholderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', c == '_':
		default:
			return false
		}
	}
	return true
}

// parseNamedParts splits the format into parts for translating with named
// arguments, in
// a single pass so that a value containing a placeholder is never substituted
// again.
func parseNamedParts(format string, placeholders map[int]*pluralPlaceholder, namedPlurals map[string]*pluralPlaceholder) []namedPart {
	var parts []namedPart
	var text strings.Builder
	rest := format
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			break
		}
		n := strings.IndexByte(rest[open+1:], '}')
		if n < 0 {
			break
		}
		end := open + 1 + n
		name := rest[open+1 : end]

		if open > 0 && rest[open-1] == '$' {
			if placeholder, ok := namedPlurals[name]; ok {
				text.WriteString(rest[:open-1])
				parts = append(parts, namedPart{text: text.String(), plural: placeholder})
				text.Reset()
				rest = rest[end+1:]
				continue
			}

			index, err := strconv.Atoi(name)
			if _, ok := placeholders[index]; err == nil && ok {
				text.WriteString(rest[:open-1])
				text.WriteString("<no arg for index " + strconv.Itoa(index) + ">")
				rest = rest[end+1:]
				continue
			}
		}

		if !isPlaceholderName(name) {
			// The brace may still open a placeholder, e.g. "{{name}".
			text.WriteString(rest[:open+1])
			rest = rest[open+1:]
			continue
		}

		text.WriteString(rest[:open])
		parts = append(parts, namedPart{text: text.String(), name: name})
		text.Reset()
		rest = rest[end+1:]
	}
	if parts == nil && text.Len() == 0 {
		return nil
	}
	text.WriteString(rest)
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
				b.WriteString(p.plural.form(m.pluralRule, count))
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
							name:  name,
							arg:   arg,
							forms: forms,
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
						name:  name,
						forms: forms,
					}
				}
				format = strings.NewReplacer(replaces...).Replace(format)
			}

			key := strings.TrimPrefix(s.Name()+"::"+k.Name(), ini.DefaultSection+"::")
			messages[key] = &Message{
				pluralRule:      rule,
				format:          format,
				positionalParts: parsePositionalParts(format, placeholders, namedPlurals),
				namedParts:      parseNamedParts(format, placeholders, namedPlurals),
			}
		}
	}

	return &Locale{
		tag:      tag,
		desc:     desc,
		messages: messages,
	}, nil
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
