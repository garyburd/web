package sqlutil

import (
	"database/sql"
	"errors"
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
// 6: disabled because two fields with name at same nesting level
// 7: not exported
// 8: recursive
// 9: scanner

type testType struct {
	Field1 string
	Field2 string `sql:"alias2"`
	anon1
	Field4 string
	Field5 string `sql:"-"`
	anon2
	field7 string
	Field9 int
}

type anon1 struct {
	Field3 string
	Field4 string
	Field6 string
}

type anon2 struct {
	Field6 string
	Field8 *testType
}

type intValue int

func (i *intValue) Scan(v interface{}) error {
	n, err := strconv.Atoi(v.(string))
	*i = intValue(n)
	return err
}

func makeScanner(v interface{}) sql.Scanner {
	switch v := v.(type) {
	case *int:
		return (*intValue)(v)
	default:
		panic("unknown type")
	}
}

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
	{&testRows{1, "Field1", "value1"}, &testType{Field1: "value1"}, nil}, // repeat previous to confirm that cache works in coverage report
	{&testRows{1, "alias2", "value2"}, &testType{Field2: "value2"}, nil},
	{&testRows{1, "field3", "value3"}, &testType{anon1: anon1{Field3: "value3"}}, nil},
	{&testRows{1, "Field4", "value4"}, &testType{Field4: "value4"}, nil},
	{&testRows{1, "Field1 Field5", "value1 value5"}, &testType{}, &badFieldError{c: "Field5", t: reflect.TypeOf(testType{})}},
	{&testRows{1, "Field1 Field6", "value1 value6"}, &testType{}, &badFieldError{c: "Field6", t: reflect.TypeOf(testType{})}},
	{&testRows{1, "Field1 field7", "value1 value7"}, &testType{}, &badFieldError{c: "field7", t: reflect.TypeOf(testType{})}},
	{&testRows{0, "Field1", "value1"}, &testType{}, sql.ErrNoRows},
	{&testRows{1, "Field9", "123"}, &testType{Field9: 123}, nil},
}

func TestScanRow(t *testing.T) {
	sc := ScanContext{MakeScanners: map[reflect.Type]func(interface{}) sql.Scanner{
		reflect.TypeOf(int(0)): makeScanner,
	}}
	for _, tt := range scanRowTests {
		t.Run(fmt.Sprintf("%s:%s", tt.rows.columns, tt.rows.values), func(t *testing.T) {
			rows := *tt.rows
			actual := reflect.New(reflect.TypeOf(tt.expected).Elem()).Interface()
			err := sc.ScanRow(&rows, actual)
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
	sc := ScanContext{}
	rows := testRows{2, "Field1 Alias2", "value1 value2"}

	var dest []testType
	if err := sc.ScanRows(&rows, &dest); err != nil {
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
	sc := ScanContext{}
	rows := testRows{2, "Field1 Alias2", "value1 value2"}

	var dest []*testType
	if err := sc.ScanRows(&rows, &dest); err != nil {
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
