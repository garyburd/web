// Copyright 2013 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cookie_test

import (
	"io"
	"net/http"

	"github.com/garyburd/web/cookie"
)

func ExampleCodec(w http.ResponseWriter, r *http.Request) {
	// Declare instance of codec at package level.
	var exampleCodec = cookie.NewCodec("example", cookie.WithSecure(true))

	// Get the value of a cookie in a request handler.
	var example string
	if err := exampleCodec.Decode(r, &example); err != nil {
		panic(err) // handle error
	}

	// Set a cookie in a response handler.
	if err := exampleCodec.Encode(w, example); err != nil {
		panic(err) // handle error
	}
	io.WriteString(w, "hello world")
}
