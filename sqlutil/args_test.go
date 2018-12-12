// Copyright 2018 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package sqlutil

import (
	"log"
	"reflect"
	"strings"
	"testing"
)

func TestArgs(t *testing.T) {
	v := &testType{
		Field1: "value1",
		anon1:  anon1{Field3: "value3"},
	}
	c := Context{MapName: strings.ToLower}
	got, err := c.Args(v, []string{"field1", "Field3"})
	if err != nil {
		log.Fatal(err)
	}
	want := []interface{}{"value1", "value3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
}
