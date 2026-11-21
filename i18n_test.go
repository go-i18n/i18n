package i18n

import (
	"fmt"
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

func TestLocale_Translate_Args(t *testing.T) {
	l, err := NewStore().AddLocale("en-US", "English", namedSource)
	assert.Nil(t, err)

	tests := []struct {
		name string
		key  string
		args Args
		want string
	}{
		{
			name: "default section",
			key:  "greeting",
			args: Args{"name": "Joe"},
			want: `Hello, Joe!`,
		},
		{
			name: "multiple placeholders",
			key:  "messages::add",
			args: Args{"path": "README.md", "repo": "gogs"},
			want: `Add 'README.md' to gogs`,
		},
		{
			name: "same placeholder twice",
			key:  "messages::twice",
			args: Args{"name": "Joe"},
			want: `Joe and Joe again`,
		},
		{
			name: "camel case and digits",
			key:  "messages::camel",
			args: Args{"externalURL": "https://gogs.io", "p50": 12, "old_name": "x"},
			want: `Visit https://gogs.io at 12 for x`,
		},
		{
			name: "placeholder without value left unchanged",
			key:  "messages::time",
			args: Args{"user": "Joe"},
			want: `Updated {time} by Joe`,
		},
		{
			name: "no fmt.Sprintf for percent signs",
			key:  "messages::percent",
			args: Args{"user": "Joe"},
			want: `Search with uid=%s for Joe, 100% done`,
		},
		{
			name: "value containing a placeholder is not substituted again",
			key:  "messages::add",
			args: Args{"path": "{repo}", "repo": "gogs"},
			want: `Add '{repo}' to gogs`,
		},
		{
			name: "braces that are not placeholders",
			key:  "messages::braces",
			args: Args{"maxFilesize": "3 MB", "not a name": "x", "": "y", "unclosed": "z"},
			want: `Size {3 MB and {not a name} and {} and {unclosed`,
		},
		{
			name: "empty args",
			key:  "messages::time",
			args: Args{},
			want: `Updated {time} by {user}`,
		},
		{
			name: "named plural one",
			key:  "messages::plural",
			args: Args{"count": 1},
			want: `Changed 1 file`,
		},
		{
			name: "named plural other",
			key:  "messages::plural",
			args: Args{"count": 3},
			want: `Changed 3 files`,
		},
		{
			name: "named plural without value",
			key:  "messages::plural",
			args: Args{},
			want: `Changed {count} <no arg for name count>`,
		},
		{
			name: "named plural with bad value",
			key:  "messages::plural",
			args: Args{"count": "many"},
			want: `Changed many <strconv.ParseFloat: parsing "many": invalid syntax>`,
		},
		{
			name: "positional verbs are kept",
			key:  "messages::positional",
			args: Args{"name": "Joe"},
			want: `My name is %s`,
		},
		{
			name: "positional plural without positional args",
			key:  "messages::both",
			args: Args{},
			want: `Changed %[1]d <no arg for index 1>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := l.Translate(test.key, test.args)
			assert.Equal(t, test.want, got)
		})
	}

	t.Run("args is not sole argument", func(t *testing.T) {
		got := l.Translate("messages::positional", "Joe", Args{"name": "x"})
		assert.Equal(t, "My name is Joe%!(EXTRA i18n.Args=map[name:x])", got)
	})

	t.Run("named plural with positional args", func(t *testing.T) {
		got := l.Translate("messages::plural", 1)
		assert.Equal(t, "Changed {count} <no arg for name count>%!(EXTRA int=1)", got)
	})
}

func TestLocale_TranslateWithFallback_Args(t *testing.T) {
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
		name string
		key  string
		args Args
		want string
	}{
		{
			name: "good",
			key:  "messages::add",
			args: Args{"path": "README.md", "repo": "gogs"},
			want: `将 'README.md' 添加到 gogs`,
		},
		{
			name: "named plural",
			key:  "messages::plural",
			args: Args{"count": 1},
			want: `变更了 1 个文件`,
		},
		{
			name: "from other source",
			key:  "messages::time",
			args: Args{"user": "Joe"},
			want: `Joe 更新于 {time}`,
		},
		{
			name: "fallback",
			key:  "greeting",
			args: Args{"name": "Joe"},
			want: `Hello, Joe!`,
		},
		{
			name: "fallback with percent signs",
			key:  "messages::percent",
			args: Args{"user": "Joe"},
			want: `Search with uid=%s for Joe, 100% done`,
		},
		{
			name: "no such key",
			key:  "messages::404",
			args: Args{"user": "Joe"},
			want: `<no such key: messages::404>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := l2.TranslateWithFallback(l1, test.key, test.args)
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

func BenchmarkLocale_Translate_Args(b *testing.B) {
	l, err := NewStore().AddLocale(
		"en-US",
		"English",
		namedSource,
	)
	assert.Nil(b, err)

	args := Args{"path": "README.md", "repo": "gogs"}
	for i := 0; i < b.N; i++ {
		l.Translate("messages::add", args)
	}
}
