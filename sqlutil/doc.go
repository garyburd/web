// Copyright 2018 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package sqlutil contains useful utilities for working with the database/sql
// package.
//
// The sql: field tag is used to override the mapping between Go struct field
// names and database column names. The format is:
//
//  `sql:name,prefix`
//
//  name - name, can be blank to use field name.
//  prefix - when used on anonymous field, the field names
//      in the embedded struct are prefixed with "prefix_"
//
// The precedence order for fields mapping to the same database column name is:
// 1) a field with lower nesting levels take precedence over a field with a
// higher nesting level 2) definition order is used for fields at the same
// nesting level. Note that (2) is different from the normal Go rules.
package sqlutil
