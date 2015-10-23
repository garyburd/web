// Copyright 2014 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cookie provides a codec for encoding and decoding values to HTTP
// cookies.
package cookie // import "github.com/garyburd/web/cookie"

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/gob"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// now is a hook for tests.
var now = time.Now

func isValidCookieValue(p []byte) bool {
	for _, b := range p {
		if b <= ' ' ||
			b >= 127 ||
			b == '"' ||
			b == ',' ||
			b == ';' ||
			b == '\\' {
			return false
		}
	}
	return true
}

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
	encode   func([]byte, interface{}) ([]byte, error)
	decode   func([]byte, interface{}) error
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
		encode:   rawEncode,
		decode:   rawDecode,
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

func (cc *Codec) validate(tv []byte, h []byte) bool {
	for i := range cc.hmacKeys {
		if hmac.Equal(cc.sign(i, tv), h) {
			return true
		}
	}
	return false
}

// Decode decodes a cookie value from a request. The value argument must be a
// pointer to a type supported by the codec.
func (cc *Codec) Decode(r *http.Request, value interface{}) error {
	sv := ""
	for _, h := range r.Header["Cookie"] {
		if m := cc.re.FindStringSubmatch(h); m != nil {
			sv = m[1]
			break
		}
	}
	if sv == "" {
		return errors.New("cookie: cookie not found")
	}

	bv := []byte(sv)

	if cc.hmacKeys != nil {

		// Check HMAC

		i := bytes.IndexByte(bv, '|')
		if i < 0 {
			return errors.New("cookie: bad value format")
		}

		h := bv[:i]
		bv = bv[i+1:]

		if !cc.validate(bv, h) {
			return errors.New("cookie: bad HMAC")
		}

		// Check expiration

		i = bytes.IndexByte(bv, '|')
		if i < 0 {
			return errors.New("cookie: bad value format")
		}

		t, err := strconv.ParseInt(string(bv[:i]), 36, 64)
		if err != nil {
			return errors.New("cookie: bad time format")
		}

		if cc.maxAge != 0 && time.Unix(t, 0).Add(cc.maxAge+time.Second).Before(now()) {
			return errors.New("cookie: expired")
		}

		bv = bv[i+1:]
	}

	return cc.decode(bv, value)
}

// Encode encodes value to a set cookie header. If value is nil, then the
// set cookie header is set to expire in the past.  The value must be a type
// supported by the codec.
func (cc *Codec) Encode(w http.ResponseWriter, value interface{}) error {

	var buf []byte

	buf = append(buf, cc.name...)
	buf = append(buf, '=')

	switch {
	case value == nil:
		buf = append(buf, '.')
	case cc.hmacKeys == nil:
		var err error
		buf, err = cc.encode(buf, value)
		if err != nil {
			return err
		}
	default:
		tv := strconv.AppendInt(nil, now().Unix(), 36)
		tv = append(tv, '|')
		var err error
		tv, err = cc.encode(tv, value)
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
	if value == nil {
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

var errTypeNotSupported = errors.New("cookie: codec does not support value type")

func rawEncode(buf []byte, value interface{}) ([]byte, error) {
	i := len(buf)
	switch value := value.(type) {
	case string:
		buf = append(buf, value...)
	case []byte:
		buf = append(buf, value...)
	default:
		return nil, errTypeNotSupported
	}
	if !isValidCookieValue(buf[i:]) {
		return nil, errors.New("invalid cookie value")
	}
	return buf, nil
}

func rawDecode(buf []byte, value interface{}) error {
	switch value := value.(type) {
	case *string:
		*value = string(buf)
	case *[]byte:
		*value = buf
	default:
		return errTypeNotSupported
	}
	return nil
}

func base64Encode(buf []byte, value interface{}) ([]byte, error) {
	var unencoded []byte
	switch value := value.(type) {
	case string:
		unencoded = []byte(value)
	case []byte:
		unencoded = value
	default:
		return nil, errTypeNotSupported
	}

	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(unencoded)))
	base64.StdEncoding.Encode(encoded, unencoded)
	return append(buf, encoded...), nil
}

func base64Decode(buf []byte, value interface{}) error {
	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(buf)))
	n, err := base64.StdEncoding.Decode(decoded, buf)
	if err != nil {
		return err
	}
	decoded = decoded[:n]

	switch value := value.(type) {
	case *string:
		*value = string(decoded)
	case *[]byte:
		*value = decoded
	default:
		return errTypeNotSupported
	}
	return nil
}

type sliceWriter struct{ buf []byte }

func (w *sliceWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func gobEncode(buf []byte, value interface{}) ([]byte, error) {
	sw := sliceWriter{buf}
	bw := base64.NewEncoder(base64.StdEncoding, &sw)
	err := gob.NewEncoder(bw).Encode(value)
	bw.Close()
	return sw.buf, err
}

func gobDecode(buf []byte, value interface{}) error {
	return gob.NewDecoder(base64.NewDecoder(base64.StdEncoding, bytes.NewReader(buf))).Decode(value)
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

// WithEncodeBase64 specifies that string and []byte cookie values should be base64 encoded.
func WithEncodeBase64() Option {
	return Option{func(cc *Codec) { cc.encode = base64Encode; cc.decode = base64Decode }}
}

// WithEncodeGob specifies that cookie values should be gob and base64 encoded.
func WithEncodeGob() Option {
	return Option{func(cc *Codec) { cc.encode = gobEncode; cc.decode = gobDecode }}
}
