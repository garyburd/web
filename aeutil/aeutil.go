// Copyright 2015 Gary Burd. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// +build appengine

// Package aeutil provides utilities for the App Engine environment.
package aeutil // import "github.com/garyburd/web/aeutil"

import (
	"appengine"

	"golang.org/x/net/context"
)

type contextKey int

const (
	appEngineContextKey contextKey = iota
)

func WithContext(ctx context.Context, aectx appengine.Context) context.Context {
	return context.WithValue(ctx, appEngineContextKey, aectx)
}

func Context(ctx context.Context) (appengine.Context, bool) {
	aectx, ok := ctx.Value(appEngineContextKey).(appengine.Context)
	return aectx, ok
}

func AppID(ctx context.Context) string {
	if aectx, ok := Context(ctx); ok {
		return appengine.AppID(aectx)
	}
	return ""
}
