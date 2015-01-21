// Copyright 2013 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"

	"golang.org/x/net/context"
)

var percentDecodeTests = []struct {
	in, out string
}{
	{in: "a", out: "a"},
	{in: "a/b", out: "a/b"},
	{in: "a%2fb", out: "a/b"},
	{in: "a%2Fb", out: "a/b"},
	{in: "a%2F", out: "a/"},
	{in: "a%2", out: ""},
	{in: "%", out: ""},
}

func TestPercentDecode(t *testing.T) {
	for _, dt := range percentDecodeTests {
		out, err := percentDecode(dt.in)
		if err != nil {
			out = ""
		}
		if dt.out != out {
			t.Errorf("deocde(%q) = %q, want %q", dt.in, out, dt.out)
		}
	}
}

func writeParams(w http.ResponseWriter, h string, params map[string]string) {
	var keys []string
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		w.Write([]byte(" "))
		w.Write([]byte(key))
		w.Write([]byte(":"))
		w.Write([]byte(params[key]))
	}
}

type routeTestHandler string

func (h routeTestHandler) Serve(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	w.Write([]byte(h))
	for _, key := range []string{"x", "y"} {
		if value, ok := Param(ctx, key); ok {
			w.Write([]byte(" "))
			w.Write([]byte(key))
			w.Write([]byte(":"))
			w.Write([]byte(value))
		}
	}
}

var routeTests = []struct {
	url    string
	method string
	status int
	body   string
}{
	{url: "/Bogus/Path", method: "GET", status: http.StatusNotFound, body: ""},
	{url: "/Bogus/Path", method: "POST", status: http.StatusNotFound, body: ""},
	{url: "/", method: "GET", status: http.StatusOK, body: "home-get"},
	{url: "/", method: "HEAD", status: http.StatusOK, body: "home-get"},
	{url: "/", method: "POST", status: http.StatusMethodNotAllowed, body: ""},
	{url: "/a", method: "GET", status: http.StatusOK, body: "a-get"},
	{url: "/a", method: "HEAD", status: http.StatusOK, body: "a-get"},
	{url: "/a", method: "POST", status: http.StatusOK, body: "a-*"},
	{url: "/a/", method: "GET", status: http.StatusNotFound, body: ""},
	{url: "/b", method: "GET", status: http.StatusOK, body: "b-get"},
	{url: "/b", method: "HEAD", status: http.StatusOK, body: "b-get"},
	{url: "/b", method: "POST", status: http.StatusOK, body: "b-post"},
	{url: "/b", method: "PUT", status: http.StatusMethodNotAllowed, body: ""},
	{url: "/c", method: "GET", status: http.StatusOK, body: "c-*"},
	{url: "/c", method: "HEAD", status: http.StatusOK, body: "c-*"},
	{url: "/d", method: "GET", status: http.StatusMovedPermanently, body: ""},
	{url: "/d/", method: "GET", status: http.StatusOK, body: "d"},
	{url: "/e", method: "GET", status: http.StatusOK, body: "e"},
	{url: "/e/", method: "GET", status: http.StatusOK, body: "e-slash"},
	{url: "/f/foo", method: "GET", status: http.StatusOK, body: "f x:foo"},
	{url: "/f/foo%2fbar", method: "GET", status: http.StatusOK, body: "f x:foo/bar"},
	{url: "/f/foo%2", method: "GET", status: http.StatusBadRequest},
	{url: "/f/foo/", method: "GET", status: http.StatusNotFound, body: ""},
	{url: "/g/foo/bar", method: "GET", status: http.StatusMovedPermanently, body: ""},
	{url: "/g/foo/bar/", method: "GET", status: http.StatusOK, body: "g x:foo y:bar"},
	{url: "/h/foo", method: "GET", status: http.StatusNotFound, body: ""},
	{url: "/h/99", method: "GET", status: http.StatusOK, body: "h x:99"},
	{url: "/h/xx/i", method: "GET", status: http.StatusMovedPermanently, body: ""},
	{url: "/h/xx/i/", method: "GET", status: http.StatusOK, body: "i"},
	{url: "/j/foo/d", method: "GET", status: http.StatusMovedPermanently, body: ""},
	{url: "/j/foo/d/", method: "GET", status: http.StatusOK, body: "j x:foo"},
	{url: "/kk/foo", method: "GET", status: http.StatusOK, body: "kk x:foo"},
}

func TestRouter(t *testing.T) {
	router := New()
	router.Add("/").Get(routeTestHandler("home-get").Serve)
	router.Add("/a").Get(routeTestHandler("a-get").Serve).Method("*", routeTestHandler("a-*").Serve)
	router.Add("/b").Get(routeTestHandler("b-get").Serve).Post(routeTestHandler("b-post").Serve)
	router.Add("/c").Method("*", routeTestHandler("c-*").Serve)
	router.Add("/d/").Get(routeTestHandler("d").Serve)
	router.Add("/e").Get(routeTestHandler("e").Serve)
	router.Add("/e/").Get(routeTestHandler("e-slash").Serve)
	router.Add("/f/<x>").Get(routeTestHandler("f").Serve)
	router.Add("/f/").Get(routeTestHandler("f").Serve)
	router.Add("/g/<x>/<y>/").Get(routeTestHandler("g").Serve)
	router.Add("/h/<x:[0-9]+>").Get(routeTestHandler("h").Serve)
	router.Add("/h/xx/i/").Get(routeTestHandler("i").Serve)
	router.Add("/j/<x>/d/").Get(routeTestHandler("j").Serve)
	router.Add("/kk/<x>").Get(routeTestHandler("kk").Serve)

	for _, rt := range routeTests {
		u, err := url.Parse(rt.url)
		if err != nil {
			u = &url.URL{Opaque: rt.url}
		}
		r := &http.Request{URL: u, RequestURI: rt.url, Method: rt.method}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != rt.status {
			t.Errorf("url=%s method=%s, status=%d, want %d", rt.url, rt.method, w.Code, rt.status)
		}
		if w.Code == http.StatusOK {
			if w.Body.String() != rt.body {
				t.Errorf("url=%s method=%s body=%q, want %q", rt.url, rt.method, w.Body.String(), rt.body)
			}
		}
	}
}

var hostRouterTests = []struct {
	host   string
	status int
	body   string
}{
	{host: "www.example.com", status: http.StatusOK, body: "www.example.com"},
	{host: "www.example.com:8080", status: http.StatusOK, body: "www.example.com"},
	{host: "foo.example.com", status: http.StatusOK, body: "*.example.com x:foo"},
	{host: "example.com", status: http.StatusOK, body: "default"},
}

func TestHostRouter(t *testing.T) {
	router := NewHostRouter()
	router.Add("www.example.com", routeTestHandler("www.example.com").Serve)
	router.Add("<x>.example.com", routeTestHandler("*.example.com").Serve)
	router.Add("<:.*>", routeTestHandler("default").Serve)

	for _, tt := range hostRouterTests {
		r := &http.Request{Host: tt.host}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tt.status {
			t.Errorf("host=%s, status=%d, want %d", tt.host, w.Code, tt.status)
		}
		if w.Code == http.StatusOK {
			if w.Body.String() != tt.body {
				t.Errorf("host=%s, body=%s, want %s", tt.host, w.Body.String(), tt.body)
			}
		}
	}
}
