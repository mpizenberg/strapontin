package main

import (
	"embed"
	"io/fs"
)

//go:embed all:web/dist
var webDist embed.FS

// frontendFS returns the embedded frontend build, or nil before the first build.
func frontendFS() fs.FS {
	sub, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		return nil
	}
	if _, err := fs.Stat(sub, "index.html"); err != nil {
		return nil
	}
	return sub
}
