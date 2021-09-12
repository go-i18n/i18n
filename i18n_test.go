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
