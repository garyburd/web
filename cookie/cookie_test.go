// Copyright 2014 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package cookie

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"testing"
	"time"
)

var cookieParseTests = []string{
	"foo=bar",
	"xxx; foo=bar; yyy",
	`xxx; foo="bar"`,
	`xxx; foo="bar"; yyy`,
	`xxxfoo=bad; foo=bar; foo=bad`,
}

func TestCookieCodecParse(t *testing.T) {
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
	cc    *Codec
	value interface{}
	attr  string
}{
	{
		NewCodec("default"),
		"hello",
		"path=/; HttpOnly",
	},
	{
		NewCodec("path", Path("/world"), HTTPOnly(false)),
		"hello",
		"path=/world",
	},
	{
		NewCodec("domain", Path(""), HTTPOnly(false), Domain("example.com")),
		"hello",
		"domain=example.com",
	},
	{
		NewCodec("secure", Path(""), HTTPOnly(false), Secure(true)),
		"hello",
		"secure",
	},
	{
		NewCodec("maxage", Path(""), HTTPOnly(false), MaxAge(time.Second)),
		"hello",
		"max-age=1; expires=Mon, 02 Jan 2006 15:04:06 GMT",
	},
	{
		NewCodec("expired", Path(""), HTTPOnly(false), MaxAge(time.Second)),
		nil,
		"max-age=-2592000; expires=Sat, 03 Dec 2005 15:04:05 GMT",
	},

	// Raw: string, []byte
	{
		NewCodec("rawstring", Path(""), HTTPOnly(false)),
		"hello",
		"",
	},
	{
		NewCodec("rawbytes", Path(""), HTTPOnly(false)),
		[]byte("hello"),
		"",
	},

	// Base64: string, []byte
	{
		NewCodec("64string", Path(""), HTTPOnly(false)),
		"hello",
		"",
	},
	{
		NewCodec("64bytes", Path(""), HTTPOnly(false)),
		[]byte("hello"),
		"",
	},

	// Gob
	{
		NewCodec("gobstring", Path(""), HTTPOnly(false), EncodeGob()),
		"hello",
		"",
	},
	{
		NewCodec("gobstruct", Path(""), HTTPOnly(false), EncodeGob()),
		&struct{ Hello string }{"world"},
		"",
	},

	// HMAC
	{
		NewCodec("hmac", Path(""), HTTPOnly(false), HMACKeys([][]byte{[]byte("key1"), []byte("key2")})),
		"hello",
		"",
	},
	{
		NewCodec("hmacMaxAge", Path(""), HTTPOnly(false), HMACKeys([][]byte{[]byte("key1"), []byte("key2")}), MaxAge(time.Second)),
		"hello",
		"max-age=1; expires=Mon, 02 Jan 2006 15:04:06 GMT",
	},
}

func TestCookieEncodeDecode(t *testing.T) {
	testTime, _ := time.Parse("Mon Jan 2 15:04:05 MST 2006", "Mon Jan 2 15:04:05 UTC 2006")
	now = func() time.Time { return testTime }
	defer func() { now = time.Now }()

	for _, tt := range cookieEncodeDecodeTests {
		s := `^` + tt.cc.name + `=([^ ;]+)`
		if tt.attr != "" {
			s += `; ` + tt.attr
		}
		s += `$`
		re, err := regexp.Compile(s)
		if err != nil {
			t.Errorf("regexp.Compile(%s) returned error %v", s, err)
			continue
		}

		w := httptest.NewRecorder()
		tt.cc.Encode(w, tt.value)
		h := w.HeaderMap.Get("Set-Cookie")

		match := re.FindStringSubmatch(h)
		if match == nil {
			t.Errorf("%s: want %q, got %q", tt.cc.name, re.String(), h)
			continue
		}
		if tt.value == nil {
			continue
		}

		r := &http.Request{Header: http.Header{"Cookie": {tt.cc.name + "=" + match[1]}}}
		v := reflect.New(reflect.TypeOf(tt.value))
		err = tt.cc.Decode(r, v.Interface())
		if err != nil {
			t.Errorf("%s: Decode(%q) returned error %v", tt.cc.name, match[1], err)
			continue
		}
		if !reflect.DeepEqual(tt.value, v.Elem().Interface()) {
			t.Errorf("%s: decode returned %v, want %v", tt.cc.name, v.Elem().Interface(), tt.value)
		}
	}
}
