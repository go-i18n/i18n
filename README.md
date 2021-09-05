# i18n

[![GitHub Workflow Status](https://img.shields.io/github/workflow/status/go-i18n/i18n/Go?logo=github&style=for-the-badge)](https://github.com/go-i18n/i18n/actions?query=workflow%3AGo)
[![codecov](https://img.shields.io/codecov/c/github/go-i18n/i18n/main?logo=codecov&style=for-the-badge)](https://codecov.io/gh/go-i18n/i18n)
[![GoDoc](https://img.shields.io/badge/GoDoc-Reference-blue?style=for-the-badge&logo=go)](https://pkg.go.dev/github.com/go-i18n/i18n?tab=doc)
[![Sourcegraph](https://img.shields.io/badge/view%20on-Sourcegraph-brightgreen.svg?style=for-the-badge&logo=sourcegraph)](https://sourcegraph.com/github.com/go-i18n/i18n)

Package i18n provides internationalization and localization for your Go applications.

## Installation

The minimum requirement of Go is **1.16**.

	go get github.com/go-i18n/i18n

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

	"github.com/go-i18n/i18n"
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

## License

This project is under the MIT License. See the [LICENSE](LICENSE) file for the full license text.
