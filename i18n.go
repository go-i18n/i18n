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
)

// Store contains a collection of locales and their descriptive names.
type Store struct {
	langs   []string
	descs   []string
	locales map[string]*Locale
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

// TODO
func (s *Store) SetLocale(lang, desc string, locale interface{}, others ...interface{}) error {
	tag, err := language.Parse(lang)
	if err != nil {
		return errors.Wrap(err, "parse lang")
	}

	file, err := ini.LoadSources(
		ini.LoadOptions{
			IgnoreInlineComment:         true,
			UnescapeValueCommentSymbols: true,
		},
		locale,
		others...,
	)
	if err != nil {
		return errors.Wrap(err, "load sources")
	}
	file.BlockMode = false

	l, err := newLocale(tag, desc, file)
	if err != nil {
		return errors.Wrap(err, "new locale")
	}
	if !s.add(l) {
		return errors.Errorf("duplicated locales for %q", lang)
	}
	return nil
}

var ErrLocaleNotFound = errors.New("locale not found")

// Locale returns the locale with the given language name.
func (s *Store) Locale(lang string) (*Locale, error) {
	l, ok := s.locales[lang]
	if !ok {
		return nil, ErrLocaleNotFound
	}
	return l, nil
}

// plural contains contents of the message for the CLDR plural forms.
type plural struct {
	zero  string
	one   string
	two   string
	few   string
	many  string
	other string
}

// Message represents a message in a locale.
type Message struct {
	format  string
	plurals map[int]*plural
}

// todo
func (m *Message) String(args ...interface{}) string {
	format := m.format
	replaces := make([]string, 0, len(m.plurals)*2)
	for k, v := range m.plurals {
		replaces = append(replaces, fmt.Sprintf("${%d}", k), v.zero) // todo
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

// TODO
func newLocale(tag language.Tag, desc string, file *ini.File) (*Locale, error) {
	const pluralsSection = "plurals"
	s := file.Section(pluralsSection)
	keys := s.Keys()
	plurals := make(map[string]*plural, len(keys))
	for _, k := range s.Keys() {
		fields := strings.SplitN(k.Name(), ".", 2)
		if len(fields) != 2 {
			continue
		}

		noun, form := fields[0], fields[1]

		p, ok := plurals[noun]
		if !ok {
			p = &plural{}
			plurals[noun] = p
		}

		switch form {
		case "zero":
			p.zero = k.String()
		case "one":
			p.one = k.String()
		case "two":
			p.two = k.String()
		case "few":
			p.few = k.String()
		case "many":
			p.many = k.String()
		case "other":
			p.other = k.String()
		}
	}

	placeholderRe := regexp.MustCompile(`\${([a-zA-z]+),\s*(\d+)}`) // e.g. ${file, 1} => ["file", "1"]

	messages := make(map[string]*Message)
	for _, s := range file.Sections() {
		for _, k := range s.Keys() {
			format := k.String()
			pluralsByIndex := make(map[int]*plural)
			if strings.Contains(format, "${") {
				matches := placeholderRe.FindAllStringSubmatch(format, -1)
				replaces := make([]string, 0, len(matches)*2)
				for _, submatch := range matches {
					placeholder := submatch[0]
					noun := submatch[1]
					index, _ := strconv.Atoi(submatch[2])

					p, ok := plurals[noun]
					if !ok {
						replaces = append(replaces, placeholder, fmt.Sprintf("<no such plural: %s>", noun))
						continue
					}

					replaces = append(replaces, placeholder, fmt.Sprintf("${%d}", index))
					pluralsByIndex[index] = p
				}
				format = strings.NewReplacer(replaces...).Replace(format)
			}

			messages[s.Name()+"::"+k.Name()] = &Message{
				format:  format,
				plurals: pluralsByIndex,
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
	return m.String(args...)
}
