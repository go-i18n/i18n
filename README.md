# i18n

[![GitHub Workflow Status](https://img.shields.io/github/checks-status/go-i18n/i18n/main?logo=github&style=for-the-badge)](https://github.com/go-i18n/i18n/actions?query=branch%3Amain)
[![codecov](https://img.shields.io/codecov/c/github/go-i18n/i18n/main?logo=codecov&style=for-the-badge)](https://codecov.io/gh/go-i18n/i18n)
[![GoDoc](https://img.shields.io/badge/GoDoc-Reference-blue?style=for-the-badge&logo=go)](https://pkg.go.dev/github.com/go-i18n/i18n?tab=doc)
[![Sourcegraph](https://img.shields.io/badge/view%20on-Sourcegraph-brightgreen.svg?style=for-the-badge&logo=sourcegraph)](https://sourcegraph.com/github.com/go-i18n/i18n)

Package i18n provides internationalization and localization for your Go applications.

## Installation

The minimum requirement of Go is **1.16**.

	go get unknwon.dev/i18n

## Getting started

```ini
# locale_en-US.ini
[plurals]
file.one = file
file.other = files

dog.one = dog
dog.other = dogs

[messages]
test1 = This patch has %[1]d changed ${file, 1} and deleted %[2]d ${file, 2}
test2 = I have %[1]d ${dog, 1}
```

```ini
# locale_zh-CN.ini
[plurals]
file.other = 文件

[messages]
test1 = 该补丁变更了 %[1]d 个${file, 1}并删除了 %[2]d 个${file, 2}
```

```go
package main

import (
	"fmt"

	"unknwon.dev/i18n"
)

func main() {
	s := i18n.NewStore()
	l1, err := s.AddLocale("en-US", "English", "locale_en-US.ini")
	// ... handler error

	l2, err := s.AddLocale("zh-CN", "简体中文", "locale_zh-CN.ini")
	// ... handler error

	fmt.Println(l1.Translate("messages::test1", 1, 2))
	// => "This patch has 1 changed file and deleted 2 files"

	fmt.Println(l2.Translate("messages::test1", 1, 2))
	// => "该补丁变更了 1 个文件并删除了 2 个文件"

	fmt.Println(l2.TranslateWithFallback(l1, "messages::test2", 1))
	// => "I have 1 dog"
}
```

### Named placeholders

Pass `i18n.Args` as the sole argument to fill `{name}` placeholders by name. A name may contain ASCII letters, digits and underscores. The message is not formatted by `fmt.Sprintf`, so literal `%` characters are kept as-is. A placeholder without a matching argument is left unchanged, and a value is never substituted again even if it contains another placeholder.

A plural can take its count from a named argument as well, e.g. `${file, count}`.

```ini
[plurals]
file.one = file
file.other = files

[messages]
test3 = Hello, {name}!
test4 = {user} changed {count} ${file, count}
```

```go
fmt.Println(l1.Translate("messages::test3", i18n.Args{"name": "Joe"}))
// => "Hello, Joe!"

fmt.Println(l1.Translate("messages::test4", i18n.Args{"user": "Joe", "count": 2}))
// => "Joe changed 2 files"
```

### Key separator

A message key is the section name and the key name joined by `::` by default, e.g. `messages::test1`. Keys in the default section (i.e. above any section) are looked up by the key name alone. Use `i18n.NewStoreWithOptions` to change the separator:

```go
s := i18n.NewStoreWithOptions(i18n.StoreOptions{KeySeparator: "."})
l, err := s.AddLocale("en-US", "English", "locale_en-US.ini")
// ... handler error

fmt.Println(l.Translate("messages.test1", 1, 2))
// => "This patch has 1 changed file and deleted 2 files"
```

## License

This project is under the MIT License. See the [LICENSE](LICENSE) file for the full license text.
