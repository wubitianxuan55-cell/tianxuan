// Package serve — mobile SPA embed support.
//
// When the mobile frontend has been built (web-mobile/dist/), a generated
// file z_embed_mobile.go will be created with go:embed directives that
// provide the embedded filesystem and index.html. If the mobile frontend
// is not built, the generated file will not exist and the mobile routes
// will simply not be registered.
//
// This file provides the fallback when z_embed_mobile.go is absent.

package serve

import "io/fs"

// tryMobileDist returns the embedded mobile filesystem if available.
// Returns (nil, false) when the mobile frontend has not been built.
// Overridden by z_embed_mobile.go when the mobile dist is present.
var tryMobileDist = func() (fs.FS, bool) {
	return nil, false
}

// getMobileIndex returns the mobile index.html content if available.
// Overridden by z_embed_mobile.go when the mobile dist is present.
var getMobileIndex = func() ([]byte, bool) {
	return nil, false
}
