package web

import (
	"io/fs"
	"net/http"
)

func StaticHandler() http.Handler {
	sub, err := fs.Sub(FS, "static")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}
