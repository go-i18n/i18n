package i18n

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStore_AddLocale(t *testing.T) {
	s := NewStore()
	_, err := s.AddLocale("en-US", "English", []byte(``))
	assert.Nil(t, err)

	t.Run("duplicated locales", func(t *testing.T) {
		_, err := s.AddLocale("en-US", "English", []byte(``))
		got := fmt.Sprintf("%v", err)
		want := `duplicated locales for "en-US"`
		assert.Equal(t, want, got)
	})

	t.Run("bad index", func(t *testing.T) {
		_, err := s.AddLocale("en-US", "English", []byte(`
[messages]
test1 = I have %[1]d ${cat, 0}
`))
		got := fmt.Sprintf("%v", err)
		want := `new locale: the smallest index is 1 but got 0 for "${cat, 0}"`
		assert.Equal(t, want, got)
	})
}

func TestStore_AddLocale_Limits(t *testing.T) {
	form := strings.Repeat("x", 2000)
	var indexes strings.Builder
	for i := 1; i <= 1100; i++ {
		indexes.WriteString("${file, " + strconv.Itoa(i) + "}")
	}
	tests := []struct {
		name    string
		message string
		wantErr string
	}{
		{name: "long text", message: strings.Repeat("x", 2<<20) + " {name} %s"},
		{name: "repeated positional plural", message: strings.Repeat("${file, 1}", 1100)},
		{name: "named plurals at limit", message: strings.Repeat("${file, n}", 524)},
		{name: "small width", message: "%-999[1]s %.3f %5.2f 100% done"},
		{
			name:    "named plurals over limit",
			message: strings.Repeat("${file, n}", 525),
			wantErr: `new locale: message "m::k": plurals may expand to more than 1048576 bytes`,
		},
		{
			name:    "distinct positional plurals",
			message: indexes.String(),
			wantErr: `new locale: message "m::k": plurals may expand to more than 1048576 bytes`,
		},
		{
			name:    "too many named placeholders",
			message: strings.Repeat("{a}", 1001),
			wantErr: `new locale: message "m::k": more than 1000 placeholders`,
		},
		{
			name:    "too many format directives",
			message: strings.Repeat("%[1]s", 1001),
			wantErr: `new locale: message "m::k": more than 1000 format directives`,
		},
		{
			name:    "large width",
			message: "%9999[1]d",
			wantErr: `new locale: message "m::k": format directive has a width or precision of more than 3 digits`,
		},
		{
			name:    "large precision",
			message: "%.1000s",
			wantErr: `new locale: message "m::k": format directive has a width or precision of more than 3 digits`,
		},
		{
			name:    "width from argument",
			message: "%[1]*[1]d",
			wantErr: `new locale: message "m::k": format directive "%[1]*" takes its width or precision from an argument`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewStore().AddLocale("en-US", "English", []byte("[plurals]\nfile.other = "+form+"\n[m]\nk = "+test.message+"\n"))
			if test.wantErr == "" {
				assert.Nil(t, err)
			} else {
				assert.EqualError(t, err, test.wantErr)
			}
		})
	}

	t.Run("width in plural form", func(t *testing.T) {
		_, err := NewStore().AddLocale("en-US", "English", []byte("[plurals]\nfile.other = %9999[1]d\n[m]\nk = ${file, 1}\n"))
		assert.EqualError(t, err, `new locale: message "m::k": format directive has a width or precision of more than 3 digits`)
	})
}

func TestStore_Locale(t *testing.T) {
	s := NewStore()
	want, err := s.AddLocale("en-US", "English", []byte(``))
	assert.Nil(t, err)

	got, err := s.Locale("en-US")
	assert.Nil(t, err)
	assert.Equal(t, want, got)

	t.Run("non-existent locale", func(t *testing.T) {
		_, err := s.Locale("zh-CN")
		got := fmt.Sprintf("%v", err)
		want := "locale not found"
		assert.Equal(t, want, got)
	})
}

var sampleSource = []byte(`
[plurals]
file.one = file
file.other = files

dog.zero = dog
dog.one = %(dog.zero)s
dog.two = dogs
dog.few = %(dog.two)s
dog.other = %(dog.two)s

[messages]
test1 = I have %[1]d changed ${file, 1} and deleted %[2]d ${file, 2}
test2 = I bought %[1]d ${cat, 1} and sold %[2]d ${dog, 2}
test3 = I have %[1]d ${dog, 10}
test4 = I have a dream
test5 = My name is %s
test6 = I have %[1]d ${dog, 1}
test7 = I have %[1]d ${dog, 99999999999999999999}
`)

func TestLocale_Translate(t *testing.T) {
	l, err := NewStore().AddLocale(
		"en-US",
		"English",
		sampleSource,
	)
	assert.Nil(t, err)

	tests := []struct {
		name string
		key  string
		args []interface{}
		want string
	}{
		{
			name: "good",
			key:  "messages::test1",
			args: []interface{}{1, 2},
			want: `I have 1 changed file and deleted 2 files`,
		},
		{
			name: "no such plural",
			key:  "messages::test2",
			args: []interface{}{1, 2},
			want: `I bought 1 <no such plural: cat> and sold 2 dogs`,
		},
		{
			name: "no arg for index",
			key:  "messages::test3",
			args: []interface{}{1},
			want: `I have 1 <no arg for index 10>`,
		},
		{
			name: "index out of range",
			key:  "messages::test7",
			args: []interface{}{1},
			want: fmt.Sprintf("I have 1 <no arg for index %d>", math.MaxInt),
		},
		{
			name: "no plural",
			key:  "messages::test4",
			args: nil,
			want: `I have a dream`,
		},
		{
			name: "no such key",
			key:  "messages::404",
			args: nil,
			want: `<no such key: messages::404>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := l.Translate(test.key, test.args...)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestLocale_TranslateWithFallback(t *testing.T) {
	s := NewStore()
	l1, err := s.AddLocale(
		"en-US",
		"English",
		sampleSource,
	)
	assert.Nil(t, err)

	l2, err := s.AddLocale(
		"zh-CN",
		"简体中文",
		[]byte(`
[plurals]
file.other = 文件

[messages]
test1 = 我变更了 %[1]d 个${file, 1}并删除了 %[2]d 个${file, 2}
`),
	)
	assert.Nil(t, err)

	tests := []struct {
		name string
		key  string
		args []interface{}
		want string
	}{
		{
			name: "good",
			key:  "messages::test1",
			args: []interface{}{1, 2},
			want: `我变更了 1 个文件并删除了 2 个文件`,
		},
		{
			name: "fallback",
			key:  "messages::test4",
			args: nil,
			want: `I have a dream`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := l2.TranslateWithFallback(l1, test.key, test.args...)
			assert.Equal(t, test.want, got)
		})
	}
}

var namedSource = []byte(`
greeting = Hello, {name}!

[plurals]
file.one = file
file.other = files

[messages]
add = Add '{path}' to {repo}
twice = {name} and {name} again
camel = Visit {externalURL} at {p50} for {old_name}
percent = Search with uid=%s for {user}, 100% done
time = Updated {time} by {user}
braces = Size {{maxFilesize} and {not a name} and {} and {unclosed
plural = Changed {count} ${file, count}
both = Changed %[1]d ${file, 1}
positional = My name is %s
`)

func TestLocale_TranslateNamed(t *testing.T) {
	l, err := NewStore().AddLocale("en-US", "English", namedSource)
	assert.Nil(t, err)

	tests := []struct {
		name  string
		key   string
		pairs []interface{}
		want  string
	}{
		{
			name:  "default section",
			key:   "greeting",
			pairs: []interface{}{"name", "Joe"},
			want:  `Hello, Joe!`,
		},
		{
			name:  "multiple placeholders",
			key:   "messages::add",
			pairs: []interface{}{"path", "README.md", "repo", "gogs"},
			want:  `Add 'README.md' to gogs`,
		},
		{
			name:  "same placeholder twice",
			key:   "messages::twice",
			pairs: []interface{}{"name", "Joe"},
			want:  `Joe and Joe again`,
		},
		{
			name:  "camel case and digits",
			key:   "messages::camel",
			pairs: []interface{}{"externalURL", "https://gogs.io", "p50", 12, "old_name", "x"},
			want:  `Visit https://gogs.io at 12 for x`,
		},
		{
			name:  "placeholder without value left unchanged",
			key:   "messages::time",
			pairs: []interface{}{"user", "Joe"},
			want:  `Updated {time} by Joe`,
		},
		{
			name:  "no fmt.Sprintf for percent signs",
			key:   "messages::percent",
			pairs: []interface{}{"user", "Joe"},
			want:  `Search with uid=%s for Joe, 100% done`,
		},
		{
			name:  "value containing a placeholder is not substituted again",
			key:   "messages::add",
			pairs: []interface{}{"path", "{repo}", "repo", "gogs"},
			want:  `Add '{repo}' to gogs`,
		},
		{
			name:  "braces that are not placeholders",
			key:   "messages::braces",
			pairs: []interface{}{"maxFilesize", "3 MB", "not a name", "x", "", "y", "unclosed", "z"},
			want:  `Size {3 MB and {not a name} and {} and {unclosed`,
		},
		{
			name:  "empty args",
			key:   "messages::time",
			pairs: []interface{}{},
			want:  `Updated {time} by {user}`,
		},
		{
			name:  "named plural one",
			key:   "messages::plural",
			pairs: []interface{}{"count", 1},
			want:  `Changed 1 file`,
		},
		{
			name:  "named plural other",
			key:   "messages::plural",
			pairs: []interface{}{"count", 3},
			want:  `Changed 3 files`,
		},
		{
			name:  "named plural without value",
			key:   "messages::plural",
			pairs: []interface{}{},
			want:  `Changed {count} <no arg for name count>`,
		},
		{
			name:  "named plural with bad value",
			key:   "messages::plural",
			pairs: []interface{}{"count", "many"},
			want:  `Changed many <strconv.ParseFloat: parsing "many": invalid syntax>`,
		},
		{
			name:  "positional verbs are kept",
			key:   "messages::positional",
			pairs: []interface{}{"name", "Joe"},
			want:  `My name is %s`,
		},
		{
			name:  "positional plural without positional args",
			key:   "messages::both",
			pairs: []interface{}{},
			want:  `Changed %[1]d <no arg for index 1>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := l.TranslateNamed(test.key, test.pairs...)
			assert.Equal(t, test.want, got)
		})
	}

	t.Run("named plural with positional args", func(t *testing.T) {
		got := l.Translate("messages::plural", 1)
		assert.Equal(t, "Changed {count} <no arg for name count>%!(EXTRA int=1)", got)
	})

	t.Run("named plural without args", func(t *testing.T) {
		got := l.Translate("messages::plural")
		assert.Equal(t, "Changed {count} <no arg for name count>", got)
	})

	t.Run("empty count", func(t *testing.T) {
		got := l.TranslateNamed("messages::plural", "count", "")
		assert.Equal(t, "Changed  <empty number>", got)
	})

	t.Run("invalid count does not inject format verbs", func(t *testing.T) {
		got := l.Translate("messages::both", "x%s%d")
		assert.Equal(t, `Changed %!d(string=x%s%d) <strconv.ParseFloat: parsing "x%s%d": invalid syntax>`, got)
	})
}

func TestLocale_TranslateNamedWithFallback(t *testing.T) {
	s := NewStore()
	l1, err := s.AddLocale("en-US", "English", namedSource)
	assert.Nil(t, err)

	l2, err := s.AddLocale(
		"zh-CN",
		"简体中文",
		[]byte(`
[plurals]
file.other = 文件

[messages]
add = 将 '{path}' 添加到 {repo}
plural = 变更了 {count} 个${file, count}
`),
		[]byte(`
[messages]
time = {user} 更新于 {time}
`),
	)
	assert.Nil(t, err)

	tests := []struct {
		name  string
		key   string
		pairs []interface{}
		want  string
	}{
		{
			name:  "good",
			key:   "messages::add",
			pairs: []interface{}{"path", "README.md", "repo", "gogs"},
			want:  `将 'README.md' 添加到 gogs`,
		},
		{
			name:  "named plural",
			key:   "messages::plural",
			pairs: []interface{}{"count", 1},
			want:  `变更了 1 个文件`,
		},
		{
			name:  "from other source",
			key:   "messages::time",
			pairs: []interface{}{"user", "Joe"},
			want:  `Joe 更新于 {time}`,
		},
		{
			name:  "fallback",
			key:   "greeting",
			pairs: []interface{}{"name", "Joe"},
			want:  `Hello, Joe!`,
		},
		{
			name:  "fallback with percent signs",
			key:   "messages::percent",
			pairs: []interface{}{"user", "Joe"},
			want:  `Search with uid=%s for Joe, 100% done`,
		},
		{
			name:  "no such key",
			key:   "messages::404",
			pairs: []interface{}{"user", "Joe"},
			want:  `<no such key: messages::404>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := l2.TranslateNamedWithFallback(l1, test.key, test.pairs...)
			assert.Equal(t, test.want, got)
		})
	}
}

func BenchmarkLocale_Translate(b *testing.B) {
	l, err := NewStore().AddLocale(
		"en-US",
		"English",
		sampleSource,
	)
	assert.Nil(b, err)

	for i := 0; i < b.N; i++ {
		l.Translate("messages::test4")
	}
}

func BenchmarkLocale_Translate_Format(b *testing.B) {
	l, err := NewStore().AddLocale(
		"en-US",
		"English",
		sampleSource,
	)
	assert.Nil(b, err)

	for i := 0; i < b.N; i++ {
		l.Translate("messages::test5", "Joe")
	}
}

func BenchmarkLocale_Translate_Plural(b *testing.B) {
	l, err := NewStore().AddLocale(
		"en-US",
		"English",
		sampleSource,
	)
	assert.Nil(b, err)

	for i := 0; i < b.N; i++ {
		l.Translate("messages::test6", 1)
	}
}

// Package-level values are boxed into interfaces on each call, as with values
// that are not constants in real code.
var benchPath, benchRepo = "README.md", "gogs"

func BenchmarkLocale_TranslateNamed(b *testing.B) {
	l, err := NewStore().AddLocale(
		"en-US",
		"English",
		namedSource,
	)
	assert.Nil(b, err)

	for i := 0; i < b.N; i++ {
		l.TranslateNamed("messages::add", "path", benchPath, "repo", benchRepo)
	}
}
