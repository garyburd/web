// Copyright 2018 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlutil

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// testRows is a mock version of sql.Rows which can only scan strings
type testRows struct {
	n       int
	columns string
	values  string
}

func (tr *testRows) Next() bool {
	tr.n--
	return tr.n >= 0
}

func (tr *testRows) Scan(dest ...interface{}) error {
	columns := strings.Fields(tr.columns)
	values := strings.Fields(tr.values)
	if len(columns) != len(dest) {
		return fmt.Errorf("expected %d dest values, go %d", len(tr.columns), len(dest))
	}
	for i := range dest {
		if s, ok := dest[i].(sql.Scanner); ok {
			if err := s.Scan(values[i]); err != nil {
				return err
			}
		} else if p, ok := dest[i].(*string); ok {
			*p = values[i]
		} else {
			return errors.New("scan dest is not a sql.Scanner or *string")
		}
	}
	return nil
}

func (tr *testRows) Columns() ([]string, error) {
	return strings.Fields(tr.columns), nil
}

var scanRowTests = []struct {
	rows     *testRows
	expected interface{}
	err      error
}{
	{&testRows{1, "Field1", "value1"}, &testType{Field1: "value1"}, nil},
	{&testRows{1, "field3", "value3"}, &testType{anon1: anon1{Field3: "value3"}}, nil}, // nested struct
	{&testRows{1, "Field4", "value4"}, &testType{Field4: "value4"}, nil},               // missing field
	{&testRows{1, "Field1 Field5", "value1 value5"}, &testType{},
		&missingFieldError{c: "Field5", t: reflect.TypeOf(testType{})}},
	{&testRows{0, "Field1", "value1"}, &testType{}, sql.ErrNoRows},
	{&testRows{1, "Field9", "123"}, &testType{Field9: 123}, nil},
}

func TestScanRow(t *testing.T) {
	c := Context{ValueScanner: valueScanner, MapName: strings.ToLower}
	for _, tt := range scanRowTests {
		t.Run(fmt.Sprintf("%s:%s", tt.rows.columns, tt.rows.values), func(t *testing.T) {
			rows := *tt.rows
			actual := reflect.New(reflect.TypeOf(tt.expected).Elem()).Interface()
			err := c.ScanRow(&rows, actual)
			if !reflect.DeepEqual(err, tt.err) {
				t.Errorf("got error %v,\nwant %v", err, tt.err)
				return
			}
			if !reflect.DeepEqual(actual, tt.expected) {
				t.Errorf("got %#v,\nwant %#v", actual, tt.expected)
			}
		})
	}
}

func TestScanRows(t *testing.T) {
	c := Context{MapName: strings.ToLower}
	rows := testRows{2, "Field1 Alias2", "value1 value2"}

	var dest []testType
	if err := c.ScanRows(&rows, &dest); err != nil {
		t.Errorf("Scan rows returned %v", err)
	}
	if len(dest) != 2 {
		t.Fatalf("got %d rows, want %d rows", len(dest), 2)
	}
	expected := testType{Field1: "value1", Field2: "value2"}
	for _, actual := range dest {
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("got %#v,\nwant %#v", actual, expected)
		}
	}
}

func TestScanRowsPtr(t *testing.T) {
	c := Context{MapName: strings.ToLower}
	rows := testRows{2, "Field1 Alias2", "value1 value2"}

	var dest []*testType
	if err := c.ScanRows(&rows, &dest); err != nil {
		t.Errorf("Scan rows returned %v", err)
	}
	if len(dest) != 2 {
		t.Fatalf("got %d rows, want %d rows", len(dest), 2)
	}
	expected := &testType{Field1: "value1", Field2: "value2"}
	for _, actual := range dest {
		if !reflect.DeepEqual(actual, expected) {
			t.Fatalf("got %#v,\nwant %#v", actual, expected)
		}
	}
}
