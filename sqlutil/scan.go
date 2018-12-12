// Copyright 2018 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlutil

import (
	"database/sql"
	"reflect"
)

// Rows is a data source for the scan methods in this package. The sql.Rows
// type from the database/sql package satisfies this interface.
type Rows interface {
	Scan(...interface{}) error
	Columns() ([]string, error)
	Next() bool
}

func (c *Context) fieldsForRows(rows Rows, t reflect.Type) ([]*Field, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	return c.fieldsForNames(columns, t)
}

// ScanRows scans multiple rows to the slice pointed to by dst. The slice
// elements must be a struct or a pointer to a struct.
func (c *Context) ScanRows(rows Rows, dst interface{}) error {
	dstv := reflect.ValueOf(dst).Elem()
	elemt := dstv.Type().Elem()
	isPtr := elemt.Kind() == reflect.Ptr
	if isPtr {
		elemt = elemt.Elem()
	}

	fields, err := c.fieldsForRows(rows, elemt)
	if err != nil {
		return err
	}
	scan := make([]interface{}, len(fields))
	for rows.Next() {
		rowp := reflect.New(elemt)
		rowv := rowp.Elem()
		for i, f := range fields {
			scan[i] = f.addr(c, rowv)
		}
		if err := rows.Scan(scan...); err != nil {
			return err
		}

		if isPtr {
			dstv.Set(reflect.Append(dstv, rowp))
		} else {
			dstv.Set(reflect.Append(dstv, rowv))
		}
	}
	return nil
}

// ScanRow scans one row to dst, a pointer to a struct.
func (c *Context) ScanRow(rows Rows, dst interface{}) error {
	if !rows.Next() {
		return sql.ErrNoRows
	}
	dstv := reflect.ValueOf(dst).Elem()
	fields, err := c.fieldsForRows(rows, dstv.Type())
	if err != nil {
		return err
	}
	scan := make([]interface{}, len(fields))
	for i, f := range fields {
		scan[i] = f.addr(c, dstv)
	}
	return rows.Scan(scan...)
}
