package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/bhavyavj/oi-assistant/internal/api/handlers"
)

func NewRouter(h *handlers.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(90 * time.Second))
	r.Use(middleware.Throttle(20))

	r.Get("/healthz", h.Health)
	r.Post("/upload-excel", h.UploadExcel)
	r.Get("/analyse", h.Analyse)
	r.Get("/fetch-nse", h.FetchNSE)
	r.Get("/ai-report", h.AIReport)

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/static/index.html")
	})
	r.Handle("/static/*", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/static/index.html")
	})

	return r
}
