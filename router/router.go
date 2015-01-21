// Copyright 2013 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package router // import "github.com/garyburd/web/router"

import (
	"errors"
	"net"
	"net/http"
	"path"
	"regexp"
	"strings"

	"golang.org/x/net/context"
)

type ErrorFn func(ctx context.Context, w http.ResponseWriter, r *http.Request, status int, err error)

type paramKey string

// Param returns the router parameter in the given context.
func Param(ctx context.Context, key string) (string, bool) {
	value, ok := ctx.Value(paramKey(key)).(string)
	return value, ok
}

func withParams(ctx context.Context, names, values []string) context.Context {
	for i, name := range names {
		if name == "" {
			continue
		}
		ctx = context.WithValue(ctx, paramKey(name), values[i])
	}
	return ctx
}

type Handler func(ctx context.Context, w http.ResponseWriter, r *http.Request)

// Router is a request handler that dispatches HTTP requests to other handlers
// using the request URI and the request method.
//
// A router has a list of routes. A route is a request path pattern and a
// collection of (method, handler) pairs.
//
// A path pattern is a string with embedded parameters. A parameter has the
// syntax:
//
//  '<' name (':' regular-expression)? '>'
//
// If the regular expression is not specified, then the regular expression
// [^/]+ is used.
//
// The pattern must begin with the character '/'.
//
// A router dispatches requests by matching the request URL path against the
// route patterns in the order that the routes were added. If a matching route
// is not found, then the router responds to the request with HTTP status 404.
//
// If a matching route is found, then the router looks for a handler using the
// request method, "GET" if the request method is "HEAD" and "*". If a handler
// is not found, then the router responds to the request with HTTP status 405.
//
// Call the PathaParams function to get the matched parameter values for a
// context.
//
// If a pattern ends with '/', then the router redirects the URL without the
// trailing slash to the URL with the trailing slash.
type Router struct {
	simpleMatch map[string]*Route
	routes      []*Route
	errfn       ErrorFn
	useURLPath  bool
}

type Route struct {
	pat      string
	addSlash bool
	cpat     *regexp.Regexp
	handlers map[string]Handler
}

var parameterRegexp = regexp.MustCompile("<([A-Za-z0-9_]*)(:[^>]*)?>")

// compilePattern compiles the pattern to a regular expression.
func compilePattern(pat string, addSlash bool, sep string) *regexp.Regexp {
	hasParam := false
	buf := make([]byte, 0, len(pat)+32)
	buf = append(buf, '^')
	for {
		a := parameterRegexp.FindStringSubmatchIndex(pat)
		if len(a) == 0 {
			buf = append(buf, regexp.QuoteMeta(pat)...)
			break
		} else {
			hasParam = true
			buf = append(buf, regexp.QuoteMeta(pat[0:a[0]])...)
			name := pat[a[2]:a[3]]
			if name == "" {
				buf = append(buf, "(?:"...)
			} else {
				buf = append(buf, "(?P<"...)
				buf = append(buf, name...)
				buf = append(buf, '>')
			}
			if a[4] >= 0 {
				buf = append(buf, pat[a[4]+1:a[5]]...)
			} else {
				buf = append(buf, "[^"...)
				buf = append(buf, sep...)
				buf = append(buf, "]+"...)
			}
			buf = append(buf, ')')
			pat = pat[a[1]:]
		}
	}
	if !hasParam {
		return nil
	}
	if addSlash {
		buf = append(buf, '?')
	}
	buf = append(buf, '$')
	return regexp.MustCompile(string(buf))
}

// Add adds a new route for the specified pattern.
func (router *Router) Add(pat string) *Route {
	if pat == "" || pat[0] != '/' {
		panic("tango: invalid route pattern " + pat)
	}
	addSlash := pat != "/" && pat[len(pat)-1] == '/'
	route := &Route{
		pat:      pat,
		handlers: make(map[string]Handler),
		addSlash: addSlash,
		cpat:     compilePattern(pat, addSlash, "/"),
	}
	if route.cpat != nil {
		router.routes = append(router.routes, route)
	} else {
		if foundRoute, _, _ := router.findRoute(pat); foundRoute != nil {
			panic("tango: pattern " + pat + " matches route " + foundRoute.pat)
		}
		router.simpleMatch[pat] = route
		if addSlash {
			pat = pat[:len(pat)-1]
			if foundRoute, _, _ := router.findRoute(pat); foundRoute == nil {
				router.simpleMatch[pat] = route
			}
		}
	}
	return route
}

// Method sets the handler for the given HTTP request method. Use "*" to match
// all methods.
func (route *Route) Method(method string, handler Handler) *Route {
	route.handlers[method] = handler
	return route
}

// Get adds a "GET" handler to the route.
func (route *Route) Get(handler Handler) *Route {
	return route.Method("GET", handler)
}

// Post adds a "POST" handler to the route.
func (route *Route) Post(handler Handler) *Route {
	return route.Method("POST", handler)
}

// UseURLPath modifies the router to use the request URL.Path field for routing
// instead of the request RequestURI field. Use this mode on App Engine or in
// other scenarios where the router is nested below a net/http ServeMux.
func (router *Router) UseURLPath() {
	router.useURLPath = true
}

// addSlash redirects to the request URL with a trailing slash.
func addSlash(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path + "/"
	if len(r.URL.RawQuery) > 0 {
		path = path + "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, path, 301)
}

func (router *Router) findRoute(path string) (*Route, []string, []string) {
	if r, ok := router.simpleMatch[path]; ok {
		return r, nil, nil
	}
	for _, r := range router.routes {
		values := r.cpat.FindStringSubmatch(path)
		if values != nil {
			return r, r.cpat.SubexpNames(), values
		}
	}
	return nil, nil, nil
}

// find the handler and path parameters using the path component of the request
// URL and the request method.
func (router *Router) findHandler(path, method string) (Handler, []string, []string) {
	route, names, values := router.findRoute(path)
	if route == nil {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) { router.errfn(ctx, w, r, 404, nil) }, nil, nil
	}
	if route.addSlash && path[len(path)-1] != '/' {
		return addSlash, nil, nil
	}
	handler := route.handlers[method]
	if handler == nil && method == "HEAD" {
		handler = route.handlers["GET"]
	}
	if handler == nil {
		handler = route.handlers["*"]
	}
	if handler == nil {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request) { router.errfn(ctx, w, r, 405, nil) }, nil, nil
	}
	return handler, names, values
}

const notHex = 127

func dehex(b byte) byte {
	switch {
	case '0' <= b && b <= '9':
		return b - '0'
	case 'a' <= b && b <= 'f':
		return b - 'a' + 10
	case 'A' <= b && b <= 'F':
		return b - 'A' + 10
	}
	return notHex
}

var errBadRequest = errors.New("bad request")

func percentDecode(s string) (string, error) {
	decode := false
	for i := 0; i < len(s); i++ {
		if s[i] == '%' {
			decode = true
			break
		}
	}
	if !decode {
		return s, nil
	}
	p := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != '%' {
			p = append(p, c)
		} else {
			if i+2 >= len(s) {
				return "", errBadRequest
			}
			a := dehex(s[i+1])
			b := dehex(s[i+2])
			if a == notHex || b == notHex {
				return "", errBadRequest
			}
			p = append(p, a<<4|b)
			i += 2
		}
	}
	return string(p), nil
}

// ServeHTTP invokes Serve with a background context.
func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	router.Serve(context.Background(), w, r)
}

// Serve dispatches the request to a registered handler.
func (router *Router) Serve(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	var (
		handler       Handler
		names, values []string
	)
	if router.useURLPath {
		handler, names, values = router.findHandler(r.URL.Path, r.Method)
	} else {
		p := r.RequestURI
		q := ""
		if i := strings.Index(p, "?"); i >= 0 {
			q = p[i:]
			p = p[:i]
		}
		cp := "/"
		if p != "" && p != "/" {
			slash := p[len(p)-1] == '/'
			cp = path.Clean(p)
			if slash {
				cp += "/"
			}
		}
		if p != cp {
			http.Redirect(w, r, cp+q, 301)
			return
		}

		handler, names, values = router.findHandler(p, r.Method)
		for i, value := range values {
			if names[i] == "" {
				continue
			}
			var err error
			values[i], err = percentDecode(value)
			if err != nil {
				router.errfn(ctx, w, r, http.StatusBadRequest, nil)
				return
			}
		}
	}

	handler(withParams(ctx, names, values), w, r)
}

// Error sets the function used to generate error responses from the router.
// The default error function calls the net/http Error function.
func (router *Router) ErrorFn(errfn ErrorFn) {
	router.errfn = errfn
}

// New allocates and initializes a new Router.
func New() *Router {
	router := &Router{simpleMatch: make(map[string]*Route)}
	router.ErrorFn(func(ctx context.Context, w http.ResponseWriter, r *http.Request, code int, err error) {
		http.Error(w, http.StatusText(code), code)
	})
	return router
}

// HostRouter is a request handler that dispatches HTTP requests to other
// handlers using the host header.
//
// A host router has a list of routes where each route is a (pattern, handler)
// pair. The router dispatches requests by matching the host header against
// the patterns in the order that the routes were registered. If a matching
// route is found, the request is dispatched to the route's handler.
//
// A pattern is a string with embedded parameters. A parameter has the syntax:
//
//  '<' name (':' regexp)? '>'
//
// If the regular expression is not specified, then the regular expression
// [^.]+ is used.
//
// Call the HostParams function to get the matched parameter values for a
// context.
type HostRouter struct {
	routes      []*hostRoute
	simpleMatch map[string]*hostRoute
	errfn       ErrorFn
}

type hostRoute struct {
	cpat    *regexp.Regexp
	handler Handler
	pat     string
}

// NewHostRouter allocates and initializes a new HostRouter.
func NewHostRouter() *HostRouter {
	return &HostRouter{
		simpleMatch: make(map[string]*hostRoute),
		errfn: func(ctx context.Context, w http.ResponseWriter, r *http.Request, status int, err error) {
			http.Error(w, http.StatusText(status), status)
		},
	}
}

// Error sets the function used to generate error responses from the router.
// The default error function calls the net/http Error function.
func (router *HostRouter) ErrorFn(errfn ErrorFn) {
	router.errfn = errfn
}

// Add adds a handler for the given pattern.
func (router *HostRouter) Add(pat string, handler Handler) {
	route := &hostRoute{
		cpat:    compilePattern(pat, false, "."),
		handler: handler,
		pat:     pat,
	}
	if route.cpat != nil {
		router.routes = append(router.routes, route)
	} else {
		if foundRoute, _, _ := router.findRoute(pat); foundRoute != nil {
			panic("tango: pattern " + pat + " matches route " + foundRoute.pat)
		}
		router.simpleMatch[pat] = route
	}
}

func (router *HostRouter) findRoute(host string) (*hostRoute, []string, []string) {
	if route, ok := router.simpleMatch[host]; ok {
		return route, nil, nil
	}
	for _, route := range router.routes {
		values := route.cpat.FindStringSubmatch(host)
		if values != nil {
			return route, route.cpat.SubexpNames(), values
		}
	}
	return nil, nil, nil
}

// ServeHTTP invokes Serve with a background context.
func (router *HostRouter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	router.Serve(context.Background(), w, r)
}

// Serve dispatches the request to a registered handler.
func (router *HostRouter) Serve(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(StripPort(r.Host))
	route, names, values := router.findRoute(host)
	if route == nil {
		router.errfn(ctx, w, r, http.StatusNotFound, nil)
		return
	}
	route.handler(withParams(ctx, names, values), w, r)
}

// StripPort removes the port specification from an address.
func StripPort(s string) string {
	if h, _, err := net.SplitHostPort(s); err == nil {
		s = h
	}
	return s
}
