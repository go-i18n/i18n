// Copyright 2014 Nick Snyder. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package plural

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewOperands(t *testing.T) {
	tests := []struct {
		input   interface{}
		wantOps *Operands
		wantErr bool
	}{
		{int64(0), &Operands{0.0, 0, 0, 0, 0, 0, 0, 0}, false},
		{int64(1), &Operands{1.0, 1, 0, 0, 0, 0, 0, 0}, false},
		{"0", &Operands{0.0, 0, 0, 0, 0, 0, 0, 0}, false},
		{"1", &Operands{1.0, 1, 0, 0, 0, 0, 0, 0}, false},
		{"1.0", &Operands{1.0, 1, 1, 0, 0, 0, 0, 0}, false},
		{"1.00", &Operands{1.0, 1, 2, 0, 0, 0, 0, 0}, false},
		{"1.3", &Operands{1.3, 1, 1, 1, 3, 3, 0, 0}, false},
		{"1.30", &Operands{1.3, 1, 2, 1, 30, 3, 0, 0}, false},
		{"1.03", &Operands{1.03, 1, 2, 2, 3, 3, 0, 0}, false},
		{"1.230", &Operands{1.23, 1, 3, 2, 230, 23, 0, 0}, false},
		{"20.0230", &Operands{20.023, 20, 4, 3, 230, 23, 0, 0}, false},
		{20.0230, nil, true},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("%#v", test.input), func(t *testing.T) {
			ops, err := NewOperands(test.input)
			if err != nil && !test.wantErr {
				assert.Nil(t, err)
			} else if err == nil && test.wantErr {
				assert.Error(t, err, "returned %#v", ops)
			}
			assert.Equal(t, test.wantOps, ops)
		})
	}
}

func BenchmarkNewOperand(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if _, err := NewOperands("1234.56780000"); err != nil {
			b.Fatal(err)
		}
	}
}
