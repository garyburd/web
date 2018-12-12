// Copyright 2018 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlutil

import (
	"database/sql"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// Context holds options and cached data for access to a DB. An application
// should create a single manager for each set of options and reuse that
// Context. Contexts are thread-safe.
type Context struct {
	// MapName maps a column or field name to a canonical name for the purpose
	// of comparing for equality. Use strings.ToLower for a case insensitive
	// database.
	MapName func(string) string

	// ValueScanner returns the scanner to use for value pointed to by dst.
	// Return nil to use the built-in database/sql scanner. If nil is returned
	// for a type, it is assumed that the function will always return nil for
	// the type.
	ValueScanner func(dst interface{}) sql.Scanner

	// ConvertValue converts a value to a type suitable for query and exec
	// arguments. Return nil to use the argument as is. If nil is returned for
	// a type, it is assumed that the function will always return nil for the
	// type.
	ConvertValue func(arg interface{}) interface{}

	fieldCache sync.Map
}

type Field struct {
	Name  string
	Type  reflect.Type
	Index []int
	Tag   reflect.StructTag

	useValueScanner bool
	useConvertValue bool
}

func (f *Field) addr(c *Context, structv reflect.Value) interface{} {
	v := structv.FieldByIndex(f.Index).Addr().Interface()
	if f.useValueScanner {
		v = c.ValueScanner(v)
	}
	return v
}

func (f *Field) value(c *Context, structv reflect.Value) interface{} {
	v := structv.FieldByIndex(f.Index).Interface()
	if f.useConvertValue {
		v = c.ConvertValue(v)
	}
	return v
}

func (c *Context) FieldsForType(t reflect.Type) []*Field {
	fields := c.fieldsForType(t)
	result := make([]*Field, len(fields))
	i := 0
	for _, f := range fields {
		result[i] = f
		i++
	}
	sort.Slice(result, func(a, b int) bool {
		fa := result[a]
		fb := result[b]
		for i := 0; i < len(fa.Index) && i < len(fb.Index); i++ {
			if fa.Index[i] < fb.Index[i] {
				return true
			} else if fa.Index[i] > fb.Index[i] {
				return false
			}
		}
		return len(fa.Index) < len(fb.Index)
	})
	return result
}

func (c *Context) fieldsForType(t reflect.Type) map[string]*Field {
	fields := make(map[string]*Field)
	c.collectFields(fields, t, make(map[reflect.Type]bool), nil, "")
	for _, f := range fields {
		if c.ValueScanner != nil {
			f.useValueScanner = c.ValueScanner(reflect.New(f.Type).Interface()) != nil
		}
		if c.ConvertValue != nil {
			f.useConvertValue = c.ConvertValue(reflect.Zero(f.Type).Interface()) != nil
		}
	}
	return fields
}

func (c *Context) mapName(s string) string {
	if c.MapName == nil {
		return s
	}
	return c.MapName(s)
}

func (c *Context) collectFields(fields map[string]*Field, t reflect.Type, visited map[reflect.Type]bool, index []int, namePrefix string) {
	// Break recursion.
	if visited[t] {
		return
	}
	visited[t] = true

	for i := 0; i < t.NumField(); i++ {
		sf := t.Field(i)
		if sf.PkgPath != "" && !sf.Anonymous {
			// Skip field if not exported and not anonymous.
			continue
		}

		var name string
		var prefix bool
		for i, p := range strings.Split(sf.Tag.Get("sql"), ",") {
			if i == 0 {
				name = p
			} else if p == "prefix" {
				prefix = true
			} else {
				panic(fmt.Errorf("sqlutil: bad tag for field %s in type %s", sf.Name, t.Name()))
			}
		}

		if name == "-" {
			// Skip field when field tag starts with "-".
			continue
		}

		if name == "" {
			name = sf.Name
		}
		name = namePrefix + name

		if sf.Anonymous {
			ft := sf.Type
			if ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				np := namePrefix
				if prefix {
					np = name + "_"
				}
				// Flatten anonymous struct field.
				c.collectFields(fields, ft, visited, append(index, i), np)
				continue
			}
		}

		if f, ok := fields[c.mapName(name)]; ok && len(f.Index) <= len(index)+1 {
			// Previous field has precedence.
			continue
		}

		f := &Field{
			Name:  name,
			Index: make([]int, len(index)+1),
			Type:  sf.Type,
			Tag:   sf.Tag,
		}
		copy(f.Index, index)
		f.Index[len(index)] = i
		fields[c.mapName(f.Name)] = f
	}
}

func (c *Context) fieldsForNames(names []string, t reflect.Type) ([]*Field, error) {
	key := struct {
		t reflect.Type
		n string
	}{
		t,
		strings.Join(names, ", "),
	}
	if v, ok := c.fieldCache.Load(key); ok {
		return v.([]*Field), nil
	}

	fields := c.fieldsForType(t)
	result := make([]*Field, len(names))
	for i, name := range names {
		f, ok := fields[c.mapName(name)]
		if !ok {
			return nil, &missingFieldError{name, t}
		}
		result[i] = f
	}
	return result, nil
}

type missingFieldError struct {
	c string
	t reflect.Type
}

func (e *missingFieldError) Error() string {
	return fmt.Sprintf("could not find field for column %s in type %s", e.c, e.t)
}
