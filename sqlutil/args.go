// Copyright 2018 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlutil

import (
	"errors"
	"reflect"
)

// Args returns the values of the fields in names from the struct pointed to by
// src.
func (c *Context) Args(src interface{}, names []string) ([]interface{}, error) {
	srcv := reflect.ValueOf(src)
	if srcv.Kind() != reflect.Ptr {
		return nil, errors.New("Args src must be pointer")
	}
	srcv = srcv.Elem()
	fields, err := c.fieldsForNames(names, srcv.Type())
	if err != nil {
		return nil, err
	}
	result := make([]interface{}, len(fields))
	for i, f := range fields {
		result[i] = f.value(c, srcv)
	}
	return result, nil
}
