// Copyright (c) 2025 NetBerth Contributors.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).
// See the LICENSE file in the project root for full license text.

package handler

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed webroot/*
var webAssets embed.FS

func UIHandler() http.Handler {
	sub, _ := fs.Sub(webAssets, "webroot")
	fileServer := http.FileServer(http.FS(sub))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API paths bypass static serving
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// Try file, fallback to index.html for SPA routing
		path := strings.TrimPrefix(r.URL.Path, "/")
		f, err := sub.Open(path)
		if err != nil {
			// SPA fallback
			r.URL.Path = "/"
		} else {
			f.Close()
		}
		fileServer.ServeHTTP(w, r)
	})
}
