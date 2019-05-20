// Copyright 2014 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cookie provides a codec for encoding and decoding values to HTTP
// cookies.
//
// The codec supports values of type int, string and []string.
package cookie // import "github.com/garyburd/web/cookie"

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// now is a hook for tests.
var now = time.Now

func isValidCookieName(s string) bool {
	for _, r := range s {
		if r <= ' ' ||
			r >= 127 ||
			strings.ContainsRune(" \t\"(),/:;<=>?@[]\\{}", r) {
			return false
		}
	}
	return true
}

// Codec encodes and decodes cookies.
type Codec struct {
	name     string
	value    string
	path     string
	domain   string
	maxAge   time.Duration
	secure   bool
	httpOnly bool
	hashFunc func() hash.Hash
	hmacKeys [][]byte
	re       *regexp.Regexp
}

type Option struct{ f func(*Codec) }

// NewCodec creates a new codec with the given options.
func NewCodec(name string, options ...Option) *Codec {
	if !isValidCookieName(name) {
		panic(name + " is not a valid cookie name")
	}
	cc := &Codec{
		name:     name,
		path:     "/",
		httpOnly: true,
		hashFunc: sha1.New,
		re:       regexp.MustCompile(`(?:; |^)` + regexp.QuoteMeta(name) + `="?([^ ",;\\]+)`),
	}
	for _, option := range options {
		option.f(cc)
	}
	return cc
}

func (cc *Codec) sign(i int, tv []byte) []byte {
	h := hmac.New(cc.hashFunc, cc.hmacKeys[i])
	io.WriteString(h, cc.name)
	io.WriteString(h, "|")
	h.Write(tv)
	sum := h.Sum(nil)
	buf := make([]byte, hex.EncodedLen(len(sum)))
	hex.Encode(buf, sum)
	return buf
}

func (cc *Codec) validate(v string, h string) bool {
	bv := []byte(v)
	bh := []byte(h)
	for i := range cc.hmacKeys {
		if hmac.Equal(cc.sign(i, bv), bh) {
			return true
		}
	}
	return false
}

// Decode decodes a cookie value from a request.
func (cc *Codec) Decode(r *http.Request, values ...interface{}) error {
	s := ""
	for _, h := range r.Header["Cookie"] {
		if m := cc.re.FindStringSubmatch(h); m != nil {
			s = m[1]
			break
		}
	}
	if s == "" {
		return errors.New("cookie: cookie not found")
	}

	if cc.hmacKeys != nil {
		var p string

		// Check HMAC

		p, s = split(s)
		if p == "" {
			return errors.New("cookie: bad value format")
		}

		if !cc.validate(s, p) {
			return errors.New("cookie: bad HMAC")
		}

		// Check expiration

		p, s = split(s)
		if p == "" {
			return errors.New("cookie: bad value format")
		}

		t, err := strconv.ParseInt(p, 36, 64)
		if err != nil {
			return errors.New("cookie: bad time format")
		}

		if cc.maxAge != 0 && time.Unix(t, 0).Add(cc.maxAge+time.Second).Before(now()) {
			return errors.New("cookie: expired")
		}
	}

	return decodeValues(s, values)
}

// Encode encodes value to a set cookie header. If value is nil, then the
// set cookie header is set to expire in the past.
func (cc *Codec) Encode(w http.ResponseWriter, values ...interface{}) error {

	var buf []byte

	buf = append(buf, cc.name...)
	buf = append(buf, '=')

	switch {
	case len(values) == 0:
		buf = append(buf, '.')
	case cc.hmacKeys == nil:
		var err error
		buf, err = encodeValues(buf, values)
		if err != nil {
			return err
		}
	default:
		tv := strconv.AppendInt(nil, now().Unix(), 36)
		tv = append(tv, '|')
		var err error
		tv, err = encodeValues(tv, values)
		if err != nil {
			return err
		}
		buf = append(buf, cc.sign(0, tv)...)
		buf = append(buf, '|')
		buf = append(buf, tv...)
	}

	if cc.path != "" {
		buf = append(buf, "; path="...)
		buf = append(buf, cc.path...)
	}

	if cc.domain != "" {
		buf = append(buf, "; domain="...)
		buf = append(buf, cc.domain...)
	}

	maxAge := cc.maxAge
	if len(values) == 0 {
		// A time in the past deletes the cookie.
		maxAge = -30 * 24 * time.Hour
	}
	if maxAge != 0 {
		buf = append(buf, "; max-age="...)
		buf = strconv.AppendInt(buf, int64(maxAge/time.Second), 10)
		buf = append(buf, "; expires="...)
		buf = append(buf, now().Add(maxAge).UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")...)
	}

	if cc.secure {
		buf = append(buf, "; secure"...)
	}

	if cc.httpOnly {
		buf = append(buf, "; HttpOnly"...)
	}

	w.Header().Add("Set-Cookie", string(buf))
	return nil
}

func (cc *Codec) SetHMACKeys(keys [][]byte) {
	cc.hmacKeys = keys
}

var errTypeNotSupported = errors.New("cookie: codec does not support value type")

// encodeBytes percent encodes bytes not allowed in cookie values, bytes used
// in percent encodings and delimiters used in this package.
func encodeBytes(buf []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch {
		case b == ' ':
			buf = append(buf, '+')
		case // byte values not allowed in cookie value
			b <= ' ' ||
				b >= 127 ||
				b == '"' ||
				b == ',' ||
				b == ';' ||
				b == '\\' ||
				// byte values with special meaning in percent encoding
				b == '+' ||
				b == '%' ||
				// value deliminter
				b == '|' ||
				// string slice delimiter
				b == '!':
			buf = append(buf, '%', "0123456789ABCDEF"[b>>4], "0123456789ABCDEF"[b&15])
		default:
			buf = append(buf, b)
		}
	}
	return buf
}

func split(s string) (string, string) {
	if i := strings.IndexByte(s, '|'); i >= 0 {
		return s[:i], s[i+1:]
	}
	return s, ""
}

func encodeValues(buf []byte, values []interface{}) ([]byte, error) {
	for i, v := range values {
		if i != 0 {
			buf = append(buf, '|')
		}
		switch v := v.(type) {
		case nil:
			// do nothing
		case string:
			buf = encodeBytes(buf, v)
		case int:
			buf = strconv.AppendInt(buf, int64(v), 36)
		case int64:
			buf = strconv.AppendInt(buf, v, 36)
		case []string:
			for j, v := range v {
				if j != 0 {
					buf = append(buf, '!')
				}
				buf = encodeBytes(buf, v)
			}
		case []int:
			for j, v := range v {
				if j != 0 {
					buf = append(buf, '!')
				}
				buf = strconv.AppendInt(buf, int64(v), 36)
			}
		case []int64:
			for j, v := range v {
				if j != 0 {
					buf = append(buf, '!')
				}
				buf = strconv.AppendInt(buf, v, 36)
			}
		default:
			return nil, fmt.Errorf("cookie: value type %s not supported", reflect.TypeOf(v))
		}
	}
	return buf, nil
}

func decodeValues(s string, values []interface{}) error {
	for len(s) > 0 && len(values) > 0 {
		var p string
		p, s = split(s)
		switch v := values[0].(type) {
		case nil:
			// do nothing
		case *string:
			var err error
			*v, err = url.QueryUnescape(p)
			if err != nil {
				return err
			}
		case *int:
			n, err := strconv.ParseInt(p, 36, 0)
			if err != nil {
				return err
			}
			*v = int(n)
		case *int64:
			n, err := strconv.ParseInt(p, 36, 64)
			if err != nil {
				return err
			}
			*v = n
		case *[]string:
			for _, q := range strings.Split(p, "!") {
				r, err := url.QueryUnescape(q)
				if err != nil {
					return err
				}
				*v = append(*v, r)
			}
		case *[]int:
			for _, q := range strings.Split(p, "!") {
				n, err := strconv.ParseInt(q, 36, 0)
				if err != nil {
					return err
				}
				*v = append(*v, int(n))
			}
		case *[]int64:
			for _, q := range strings.Split(p, "!") {
				n, err := strconv.ParseInt(q, 36, 64)
				if err != nil {
					return err
				}
				*v = append(*v, n)
			}
		default:
			return fmt.Errorf("cookie: value type %s not supported", reflect.TypeOf(v))
		}
		values = values[1:]
	}
	return nil
}

// WithPath sets the cookie path attribute. The path must either be "" or start
// with a '/'.  The default value for path is "/". If the path is "", then the
// path attribute is not included in the header value.
func WithPath(path string) Option { return Option{func(cc *Codec) { cc.path = path }} }

// WithDomain sets the cookie domain attribute. If the host is "", then the domain
// attribute is not included in the header value.
func WithDomain(domain string) Option { return Option{func(cc *Codec) { cc.domain = domain }} }

// WithMaxAge specifies the maximum age for a cookie. If the maximum age is 0, then
// the expiration time is not included in the header value and the browser will
// handle the cookie as a "session" cookie. To support Internet Explorer, the
// maximum age is also rendered as an absolute expiration time.
func WithMaxAge(maxAge time.Duration) Option { return Option{func(cc *Codec) { cc.maxAge = maxAge }} }

// WithSecure sets the secure attribute.
func WithSecure(secure bool) Option { return Option{func(cc *Codec) { cc.secure = secure }} }

// WithHTTPOnly sets the httponly attribute. The default value for the httponly attribute is true.
func WithHTTPOnly(httpOnly bool) Option { return Option{func(cc *Codec) { cc.httpOnly = httpOnly }} }

// WithHashFunc sets the hash algorithm used to create HMAC. The default value for
// the hash algorithm is crypto/sha1.New.
func WithHashFunc(f func() hash.Hash) Option { return Option{func(cc *Codec) { cc.hashFunc = f }} }

// WithHMACKeys specifies the keys for signing cookies. Multiple keys are allowed
// to support key rotation. Cookies are signed with the first key. If keys is
// nil, then the cookie is not signed.
func WithHMACKeys(keys [][]byte) Option { return Option{func(cc *Codec) { cc.hmacKeys = keys }} }
