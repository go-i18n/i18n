// Copyright 2021 Unknwon. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package i18n

import (
	"github.com/pkg/errors"
	"golang.org/x/text/language"
	"gopkg.in/ini.v1"
)

// TODO
type Store struct {
	langs   []string
	descs   []string
	locales map[string]*Locale
}

// TODO
func (s *Store) add(l *Locale) bool {
	if _, ok := s.locales[l.Lang()]; ok {
		return false
	}

	// lc.id = len(d.langs)
	s.langs = append(s.langs, l.Lang())
	s.descs = append(s.descs, l.Desc())
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

// todo
func (s *Store) Locale(lang string) (*Locale, error) {
	l, ok := s.locales[lang]
	if !ok {
		return nil, ErrLocaleNotFound
	}
	return l, nil
}

// todo
type Message struct {
	// todo
}

// todo
func (m *Message) String(args ...interface{}) string {
	// todo: handle plural
	return "<not yet implemented>"
}

// TODO
type Locale struct {
	tag      language.Tag
	desc     string
	file     *ini.File
	messages map[string]*Message
}

// TODO
func newLocale(tag language.Tag, desc string, file *ini.File) (*Locale, error) {
	l := &Locale{
		tag:  tag,
		desc: desc,
		file: file,
	}

	// TODO: pre-processing plural

	return l, nil
}

// TODO
func (l *Locale) Lang() string {
	return l.tag.String()
}

// TODO
func (l *Locale) Desc() string {
	return l.desc
}

// todo
func (l *Locale) Tr(key string, args ...interface{}) string {
	m, ok := l.messages[key]
	if !ok {
		return key
	}
	return m.String(args...)
}
