// Copyright 2014 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cookie

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

var encodeDecodeValueTests = []struct {
	values []interface{}
}{
	{
		values: []interface{}{1},
	},
	{
		values: []interface{}{1, nil, 100},
	},
	{
		values: []interface{}{"hello"},
	},
	{
		values: []interface{}{"hello|world|"},
	},
	{
		values: []interface{}{"hello world"},
	},
	{
		values: []interface{}{nil},
	},
	{
		values: []interface{}{[]string{"Hello!", "World!"}},
	},
}

func TestEncodeDecodeValue(t *testing.T) {
	for _, tt := range encodeDecodeValueTests {
		p, err := encodeValues(nil, tt.values)
		if err != nil {
			t.Errorf("encodeValues(nil, %#v) returned error %v", tt.values, err)
			continue
		}
		var pvalues []interface{}
		for _, v := range tt.values {
			var pv interface{}
			if v != nil {
				pv = reflect.New(reflect.TypeOf(v)).Interface()
			}
			pvalues = append(pvalues, pv)
		}
		err = decodeValues(string(p), pvalues)
		if err != nil {
			t.Errorf("decodeValues(%q, values) returned error %v", p, err)
			continue
		}
		var values []interface{}
		for _, pv := range pvalues {
			var v interface{}
			if pv != nil {
				v = reflect.ValueOf(pv).Elem().Interface()
			}
			values = append(values, v)
		}
		if !reflect.DeepEqual(values, tt.values) {
			t.Errorf("decodeValues(%q, ...) returned %#v, want %#v", p, values, tt.values)
		}
	}
}

var cookieParseTests = []string{
	"foo=bar",
	"xxx; foo=bar; yyy",
	`xxx; foo="bar"`,
	`xxx; foo="bar"; yyy`,
	`xxxfoo=bad; foo=bar; foo=bad`,
}

func TestCookieParse(t *testing.T) {
	for _, tt := range cookieParseTests {
		r := &http.Request{Header: http.Header{"Cookie": {tt}}}
		fooCookie := NewCodec("foo")
		var s string
		fooCookie.Decode(r, &s)
		if s != "bar" {
			t.Errorf("could not parse cookie foo from %q, got %q, want %q", tt, "bar", s)
		}
	}
}

var cookieEncodeDecodeTests = []struct {
	cc       *Codec
	h        string
	novalues bool
}{
	{
		cc: NewCodec("default"),
		h:  "default=foo; path=/; HttpOnly",
	},
	{
		cc: NewCodec("path", WithPath("/world"), WithHTTPOnly(false)),
		h:  "path=foo; path=/world",
	},
	{
		cc: NewCodec("domain", WithPath(""), WithHTTPOnly(false), WithDomain("example.com")),
		h:  "domain=foo; domain=example.com",
	},
	{
		cc: NewCodec("secure", WithPath(""), WithHTTPOnly(false), WithSecure(true)),
		h:  "secure=foo; secure",
	},
	{
		cc: NewCodec("maxage", WithPath(""), WithHTTPOnly(false), WithMaxAge(time.Second)),
		h:  "maxage=foo; max-age=1; expires=Mon, 02 Jan 2006 15:04:06 GMT",
	},
	{
		cc:       NewCodec("expired", WithPath(""), WithHTTPOnly(false), WithMaxAge(time.Second)),
		h:        "expired=.; max-age=-2592000; expires=Sat, 03 Dec 2005 15:04:05 GMT",
		novalues: true,
	},
	{
		cc: NewCodec("hmac", WithPath(""), WithHTTPOnly(false), WithHMACKeys([][]byte{[]byte("key1"), []byte("key2")})),
		h:  "hmac=b1d674f6bdcc43616c8460025d0f1c774b43e774|ish0it|foo",
	},
	{
		cc: NewCodec("hmacMaxAge", WithPath(""), WithHTTPOnly(false), WithHMACKeys([][]byte{[]byte("key1"), []byte("key2")}), WithMaxAge(time.Second)),
		h:  "hmacMaxAge=49d5fc4c42969d0f8a6ad7a629034a7e8f7b1f63|ish0it|foo; max-age=1; expires=Mon, 02 Jan 2006 15:04:06 GMT",
	},
}

func TestCookieEncodeDecode(t *testing.T) {
	testTime, _ := time.Parse("Mon Jan 2 15:04:05 MST 2006", "Mon Jan 2 15:04:05 UTC 2006")
	now = func() time.Time { return testTime }
	defer func() { now = time.Now }()

	for _, tt := range cookieEncodeDecodeTests {
		w := httptest.NewRecorder()
		var values []interface{}
		if !tt.novalues {
			values = []interface{}{"foo"}
		}
		tt.cc.Encode(w, values...)
		h := w.HeaderMap.Get("Set-Cookie")
		if h != tt.h {
			t.Errorf("%s: got %q, want %q", tt.cc.name, h, tt.h)
			continue
		}
		if tt.novalues {
			continue
		}
		if i := strings.IndexByte(h, ';'); i >= 0 {
			h = h[:i]
		}
		r := &http.Request{Header: http.Header{"Cookie": {h}}}
		var s string
		err := tt.cc.Decode(r, &s)
		if err != nil {
			t.Errorf("Decode %q returned error %v", h, err)
			continue
		}
		if s != "foo" {
			t.Errorf("Decode %q returned %q, want 'foo'", h, s)
		}
	}
}
