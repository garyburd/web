package sqlutil

import (
	"database/sql"
	"fmt"
	"reflect"
	"strings"
	"sync"
)

type Rows interface {
	Scan(...interface{}) error
	Columns() ([]string, error)
	Next() bool
}

type ScanContext struct {
	MakeScanners map[reflect.Type]func(dst interface{}) sql.Scanner

	cache sync.Map
}

type field struct {
	name        string
	typ         reflect.Type
	index       []int
	makeScanner func(interface{}) sql.Scanner
}

func (sc *ScanContext) fieldsForType(t reflect.Type) []*field {
	fields := sc.collectFields(nil, t, make(map[reflect.Type]bool), make(map[string]int), nil)
	return fields
}

func (sc *ScanContext) collectFields(fields []*field, t reflect.Type, visited map[reflect.Type]bool, depth map[string]int, index []int) []*field {
	// Break recursion.
	if visited[t] {
		return fields
	}
	visited[t] = true

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" && !sf.Anonymous {
			// Skip field if not exported and not anonymous.
			continue
		}

		var name string
		for i, p := range strings.Split(sf.Tag.Get("sql"), ",") {
			if i == 0 {
				name = p
			} else {
				panic(fmt.Errorf("sqlutil: bad tag for field %s in type %s", sf.Name, t.Name()))
			}
		}

		if name == "-" {
			// Skip field when field tag starts with "-".
			continue
		}

		ft := sf.Type
		if ft.Name() == "" && ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}

		if name == "" && sf.Anonymous && ft.Kind() == reflect.Struct {
			// Flatten anonymous struct field.
			fields = sc.collectFields(fields, ft, visited, depth, append(index, i))
			continue
		}

		if name == "" {
			name = sf.Name
		}

		name = strings.ToLower(name) // names are case insensitive

		// Check for name collisions.
		d, found := depth[name]
		if !found {
			d = 65535
		}
		if len(index) == d {
			// There is another field with same name and same depth.
			// Remove that field and skip this field.
			j := 0
			for i := 0; i < len(fields); i++ {
				if name != fields[i].name {
					fields[j] = fields[i]
					j++
				}
			}
			fields = fields[:j]
			continue
		}
		depth[name] = len(index)

		f := &field{
			name:        name,
			index:       make([]int, len(index)+1),
			typ:         sf.Type,
			makeScanner: sc.MakeScanners[sf.Type],
		}
		copy(f.index, index)
		f.index[len(index)] = i
		fields = append(fields, f)
	}
	return fields
}

type badFieldError struct {
	c string
	t reflect.Type
}

func (e *badFieldError) Error() string {
	return fmt.Sprintf("could not find field for column %s in type %s", e.c, e.t)
}

func (sc *ScanContext) valueFns(rows Rows, t reflect.Type) ([]func(v reflect.Value) interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	key := struct {
		t reflect.Type
		c string
	}{
		t,
		strings.Join(columns, "\x00"),
	}
	if v, ok := sc.cache.Load(key); ok {
		return v.([]func(v reflect.Value) interface{}), nil
	}

	m := make(map[string]*field)
	for _, f := range sc.fieldsForType(t) {
		m[strings.ToLower(f.name)] = f
	}

	fns := make([]func(reflect.Value) interface{}, len(columns))
	for i, c := range columns {
		f, ok := m[strings.ToLower(c)]
		if !ok {
			return nil, &badFieldError{c, t}
		}
		var fn func(v reflect.Value) interface{}
		index := f.index
		if makeScanner := f.makeScanner; makeScanner != nil {
			fn = func(v reflect.Value) interface{} { return makeScanner(v.FieldByIndex(index).Addr().Interface()) }
		} else {
			fn = func(v reflect.Value) interface{} { return v.FieldByIndex(index).Addr().Interface() }
		}
		fns[i] = fn
	}
	sc.cache.Store(key, fns)
	return fns, nil
}

func (sc *ScanContext) ScanRows(rows Rows, dest interface{}) error {
	destv := reflect.ValueOf(dest).Elem()
	elemt := destv.Type().Elem()
	isPtr := elemt.Kind() == reflect.Ptr
	if isPtr {
		elemt = elemt.Elem()
	}

	fns, err := sc.valueFns(rows, elemt)
	if err != nil {
		return err
	}
	scan := make([]interface{}, len(fns))
	for rows.Next() {
		rowp := reflect.New(elemt)
		rowv := rowp.Elem()
		for i, fn := range fns {
			scan[i] = fn(rowv)
		}
		if err := rows.Scan(scan...); err != nil {
			return err
		}

		if isPtr {
			destv.Set(reflect.Append(destv, rowp))
		} else {
			destv.Set(reflect.Append(destv, rowv))
		}
	}
	return nil
}

func (sc *ScanContext) ScanRow(rows Rows, dest interface{}) error {
	if !rows.Next() {
		return sql.ErrNoRows
	}
	destv := reflect.ValueOf(dest).Elem()
	fns, err := sc.valueFns(rows, destv.Type())
	if err != nil {
		return err
	}
	scan := make([]interface{}, len(fns))
	for i, fn := range fns {
		scan[i] = fn(destv)
	}
	return rows.Scan(scan...)
}