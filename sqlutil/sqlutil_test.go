// Copyright 2018 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlutil

import (
	"database/sql"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// Fields used in test:
// 1: plain field
// 2: aliased field
// 3: field in embedded type
// 4: field in outer type overrides field in inner type
// 5: disabled with -
// 6: Two fields with same name at same nesting level
// 7: not exported
// 8: recursive
// 9: scanner
// 10: prefixed field

type testType struct {
	Field1 string
	Field2 string `sql:"alias2"`
	anon1
	Field4 string
	Field5 string `sql:"-"`
	anon2
	field7 string
	Field9 intValue
	anon3  `sql:"x,prefix"`
}

type anon1 struct {
	Field3  string
	Field4  string
	Field6  string
	Field10 string
}

type anon2 struct {
	Field6 string
	Field8 *testType
}

type anon3 struct {
	Field10 string
}

type intValue int

func (i *intValue) Scan(v interface{}) error {
	n, err := strconv.Atoi(v.(string))
	*i = intValue(n)
	return err
}

func valueScanner(v interface{}) sql.Scanner {
	switch v := v.(type) {
	case *int:
		return (*intValue)(v)
	default:
		return nil
	}
}

func TestFields(t *testing.T) {
	want := []*Field{
		{Name: "Field1", Type: reflect.TypeOf(""), Index: []int{0}},
		{Name: "alias2", Type: reflect.TypeOf(""), Index: []int{1}, Tag: `sql:"alias2"`},
		{Name: "Field3", Type: reflect.TypeOf(""), Index: []int{2, 0}},
		{Name: "Field6", Type: reflect.TypeOf(""), Index: []int{2, 2}},
		{Name: "Field10", Type: reflect.TypeOf(""), Index: []int{2, 3}},
		{Name: "Field4", Type: reflect.TypeOf(""), Index: []int{3}},
		{Name: "Field8", Type: reflect.TypeOf(&testType{}), Index: []int{5, 1}},
		{Name: "Field9", Type: reflect.TypeOf(intValue(0)), Index: []int{7}},
		{Name: "x_Field10", Type: reflect.TypeOf(""), Index: []int{8, 0}},
	}

	var c Context
	got := c.FieldsForType(reflect.TypeOf(testType{}))
	if !reflect.DeepEqual(got, want) {
		var message strings.Builder
		for _, x := range []struct {
			what   string
			fields []*Field
		}{{"got", got}, {"want", want}} {
			fmt.Fprintf(&message, "%s:\n", x.what)
			for _, f := range x.fields {
				fmt.Fprintf(&message, "\t%+v\n", *f)
			}
		}
		t.Fatal(message.String())
	}
}
