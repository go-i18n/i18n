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
	"gopkg.in/ini.v1"

	"unknwon.dev/i18n/internal/plural"
)

// DefaultKeySeparator is the default separator between the section name and
// the key name of a message key, e.g. "messages::test1".
const DefaultKeySeparator = "::"

// StoreOptions contains options to customize a Store.
type StoreOptions struct {
	// KeySeparator is the separator between the section name and the key name of
	// a message key, e.g. "." makes the key "test1" in the section "messages" to
	// be looked up as "messages.test1". Default is DefaultKeySeparator.
	KeySeparator string
}

// Store contains a collection of locales and their descriptive names.
type Store struct {
	langs        []string
	descs        []string
	locales      map[string]*Locale
	rules        plural.Rules
	keySeparator string
}

// NewStore initializes and returns a new Store with default options.
func NewStore() *Store {
	return NewStoreWithOptions(StoreOptions{})
}

// NewStoreWithOptions initializes and returns a new Store with given options.
func NewStoreWithOptions(opts StoreOptions) *Store {
	if opts.KeySeparator == "" {
		opts.KeySeparator = DefaultKeySeparator
	}
	return &Store{
		locales:      make(map[string]*Locale),
		rules:        plural.DefaultRules(),
		keySeparator: opts.KeySeparator,
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
// https://ini.unknwon.io/docs/howto/load_data_sources.
func (s *Store) AddLocale(lang, desc string, source interface{}, others ...interface{}) (*Locale, error) {
	tag, err := language.Parse(lang)
	if err != nil {
		return nil, errors.Wrap(err, "parse lang")
	}

	file, err := ini.LoadSources(
		ini.LoadOptions{
			IgnoreInlineComment:         true,
			UnescapeValueCommentSymbols: true,
		},
		source,
		others...,
	)
	if err != nil {
		return nil, errors.Wrap(err, "load sources")
	}
	file.BlockMode = false // We only read from the file

	rule := s.rules[tag]
	if rule == nil {
		base, confidence := tag.Base()
		if confidence != language.No {
			rule = s.rules[language.MustParse(base.String())]
		}
	}

	l, err := newLocale(tag, desc, rule, file, s.keySeparator)
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

// Args contains named arguments to fill "{name}" placeholders of a message.
// It takes effect only when passed as the sole argument for translating, e.g.
// l.Translate("messages::test1", i18n.Args{"name": "Joe"}).
type Args map[string]interface{}

// Message represents a message in a locale.
type Message struct {
	pluralRule   *plural.Rule
	format       string
	placeholders map[int]*pluralPlaceholder
	// namedPlurals is keyed by the content of the placeholder within the format,
	// e.g. "file:count" for "${file:count}" that was parsed from "${file, count}".
	namedPlurals map[string]*pluralPlaceholder
}

// Translate translates the message with the supplied list of arguments. When
// the sole argument is Args, each "{name}" placeholder is replaced by the value
// of the argument with the same name, and the message is not formatted by
// fmt.Sprintf. A placeholder without a matching argument is left unchanged.
func (m *Message) Translate(args ...interface{}) string {
	if len(args) == 0 {
		return m.format
	}
	if len(args) == 1 {
		if named, ok := args[0].(Args); ok {
			return m.translateArgs(named)
		}
	}
	if len(m.placeholders) == 0 && len(m.namedPlurals) == 0 {
		return fmt.Sprintf(m.format, args...)
	}

	// NOTE: strings.NewReplacer makes >3x more allocations and 5x slower than strings.Replace.
	//  For strings.NewReplacer:
	//  	BenchmarkLocale_Translate_Plural-16    	  987433	      1097 ns/op	    2585 B/op	      10 allocs/op
	//  For strings.Replace:
	//  	BenchmarkLocale_Translate_Plural-16    	 4316941	       285.3 ns/op	      80 B/op	       3 allocs/op

	format := m.format
	for index, placeholder := range m.placeholders {
		if len(args) < index {
			format = strings.Replace(format, placeholder.name, fmt.Sprintf("<no arg for index %d>", index), 1)
			continue
		}

		format = strings.Replace(format, placeholder.name, placeholder.form(m.pluralRule, args[index-1]), 1)
	}
	for _, placeholder := range m.namedPlurals {
		format = strings.ReplaceAll(format, placeholder.name, fmt.Sprintf("<no arg for name %s>", placeholder.arg))
	}
	return fmt.Sprintf(format, args...)
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

// translateArgs translates the message with named arguments in a single pass,
// so that a value containing a placeholder is never substituted again.
func (m *Message) translateArgs(args Args) string {
	format := m.format
	var b strings.Builder
	b.Grow(len(format))
	for {
		open := strings.IndexByte(format, '{')
		if open < 0 {
			break
		}
		n := strings.IndexByte(format[open+1:], '}')
		if n < 0 {
			break
		}
		end := open + 1 + n
		name := format[open+1 : end]

		if open > 0 && format[open-1] == '$' {
			if placeholder, ok := m.namedPlurals[name]; ok {
				b.WriteString(format[:open-1])
				count, ok := args[placeholder.arg]
				if ok {
					b.WriteString(placeholder.form(m.pluralRule, count))
				} else {
					b.WriteString("<no arg for name " + placeholder.arg + ">")
				}
				format = format[end+1:]
				continue
			}

			index, err := strconv.Atoi(name)
			if _, ok := m.placeholders[index]; err == nil && ok {
				b.WriteString(format[:open-1])
				fmt.Fprintf(&b, "<no arg for index %d>", index)
				format = format[end+1:]
				continue
			}
		}

		if !isPlaceholderName(name) {
			// The brace may still open a placeholder, e.g. "{{name}".
			b.WriteString(format[:open+1])
			format = format[open+1:]
			continue
		}

		value, ok := args[name]
		if ok {
			b.WriteString(format[:open])
			fmt.Fprint(&b, value)
		} else {
			b.WriteString(format[:end+1])
		}
		format = format[end+1:]
	}
	b.WriteString(format)
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

// newLocale creates a new Locale with given language tag, description, the raw
// locale file and the separator between section and key names. The "[plurals]"
// section is reserved to define all plurals.
func newLocale(tag language.Tag, desc string, rule *plural.Rule, file *ini.File, keySeparator string) (*Locale, error) {
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

					index, err := strconv.Atoi(submatch[2])
					if err != nil {
						if !ok {
							replaces = append(replaces, placeholder, fmt.Sprintf("<no such plural: %s>", noun))
							continue
						}

						arg := submatch[2]
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

			key := k.Name()
			if s.Name() != ini.DefaultSection {
				key = s.Name() + keySeparator + key
			}
			messages[key] = &Message{
				pluralRule:   rule,
				format:       format,
				placeholders: placeholders,
				namedPlurals: namedPlurals,
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

// Translate uses the locale to translate the message of the given key. Pass
// Args as the sole argument to fill named placeholders, see Message.Translate.
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
