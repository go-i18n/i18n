// Copyright 2021 Unknwon. All rights reserved.
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

	"github.com/go-i18n/i18n/internal/plural"
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

	l, err := newLocale(tag, desc, rule, file)
	if err != nil {
		return nil, errors.Wrap(err, "new locale")
	}
	if !s.add(l) {
		return nil, errors.Errorf("duplicated locales for %q", lang)
	}
	return l, nil
}

// Locale returns the locale with the given language name.
func (s *Store) Locale(lang string) (*Locale, error) {
	l, ok := s.locales[lang]
	if !ok {
		return nil, errors.Errorf("locale not found for %q", lang)
	}
	return l, nil
}

type pluralPlaceholder struct {
	name  string
	forms map[plural.Form]string
}

// Message represents a message in a locale.
type Message struct {
	pluralRule   *plural.Rule
	format       string
	placeholders map[int]*pluralPlaceholder
}

// Translate translates the message with the supplied list of arguments.
func (m *Message) Translate(args ...interface{}) string {
	if len(args) == 0 {
		return m.format
	}
	if len(m.placeholders) == 0 {
		return fmt.Sprintf(m.format, args...)
	}

	format := m.format
	replaces := make([]string, 0, len(m.placeholders)*2)
	for index, placeholder := range m.placeholders {
		if len(args) < index {
			replaces = append(replaces, placeholder.name, fmt.Sprintf("<no arg for index %d>", index))
			continue
		}

		ops, err := plural.NewOperands(args[index-1])
		if err != nil {
			replaces = append(replaces, placeholder.name, fmt.Sprintf("<%v>", err))
			continue
		}

		form := plural.Other
		if m.pluralRule != nil {
			form = m.pluralRule.PluralFormFunc(ops)
		}

		replaces = append(replaces, placeholder.name, placeholder.forms[form])
	}
	format = strings.NewReplacer(replaces...).Replace(format)
	return fmt.Sprintf(format, args...)
}

// Locale represents a locale with target language and a collection of messages.
type Locale struct {
	tag      language.Tag
	desc     string
	messages map[string]*Message
}

var placeholderRe = regexp.MustCompile(`\${([a-zA-z]+),\s*(\d+)}`) // e.g. ${file, 1} => ["file", "1"]

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

			format := k.String()
			if strings.Contains(format, "${") {
				matches := placeholderRe.FindAllStringSubmatch(format, -1)
				replaces := make([]string, 0, len(matches)*2)
				placeholders = make(map[int]*pluralPlaceholder, len(matches))
				for _, submatch := range matches {
					placeholder := submatch[0]
					noun := submatch[1]
					index, _ := strconv.Atoi(submatch[2])
					if index < 1 {
						return nil, errors.Errorf("the smallest index is 1 but got %d for %q", index, placeholder)
					}

					forms, ok := pluralForms[noun]
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

			messages[s.Name()+"::"+k.Name()] = &Message{
				pluralRule:   rule,
				format:       format,
				placeholders: placeholders,
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
	m, ok := l.messages[key]
	if !ok {
		return fmt.Sprintf("<no such key: %s>", key)
	}
	return m.Translate(args...)
}
