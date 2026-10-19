package main

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:portalui
var portalFiles embed.FS

func portalStatic() http.Handler {
	root, err := fs.Sub(portalFiles, "portalui")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if name == "" || name == "." {
			name = "index.html"
		}
		served := name
		data, err := fs.ReadFile(root, name)
		if err != nil && !strings.Contains(path.Base(name), ".") {
			for _, candidate := range []string{name + ".html", path.Join(name, "index.html")} {
				data, err = fs.ReadFile(root, candidate)
				if err == nil {
					served = candidate
					break
				}
			}
		}
		if err != nil && !strings.Contains(path.Base(name), ".") {
			data, err = fs.ReadFile(root, "index.html")
			served = "index.html"
		}
		if err != nil {
			http.NotFound(w, r)
			return
		}
		switch {
		case strings.HasSuffix(served, ".js"):
			w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		case strings.HasSuffix(served, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(served, ".html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		case strings.HasSuffix(served, ".svg"):
			w.Header().Set("Content-Type", "image/svg+xml")
		case strings.HasSuffix(served, ".png"):
			w.Header().Set("Content-Type", "image/png")
		case strings.HasSuffix(served, ".woff2"):
			w.Header().Set("Content-Type", "font/woff2")
		}
		_, _ = w.Write(data)
	})
}
