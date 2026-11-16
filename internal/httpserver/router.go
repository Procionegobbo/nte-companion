package httpserver

import (
	"io/fs"
	"net/http"
)

// New wires every route together. regolamentoImgFS serves the chapter
// diagrams (a sub-filesystem rooted at content/regolamento/img), staticFS
// serves CSS/JS/fonts (a sub-filesystem rooted at web/static).
func New(pages *PageHandler, chatHandler http.Handler, regolamentoImgFS, staticFS fs.FS, trustProxyHeaders bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", pages.Home)
	mux.HandleFunc("GET /regolamento/{slug}", pages.Chapter)
	mux.Handle("GET /regolamento/img/", http.StripPrefix("/regolamento/img/", http.FileServerFS(regolamentoImgFS)))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))
	mux.Handle("POST /api/chat", chatHandler)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})

	var handler http.Handler = mux
	handler = withClientIP(trustProxyHeaders)(handler)
	handler = withLogging(handler)
	handler = withRecover(handler)
	return handler
}
