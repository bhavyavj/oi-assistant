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

	r.Get("/healthz", h.Health)
	r.Post("/upload-excel", h.UploadExcel)
	r.Get("/analyse", h.Analyse)

	return r
}
