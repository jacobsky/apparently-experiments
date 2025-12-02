package server

import (
	"apparently-typing/internal/views/anim"
	"apparently-typing/internal/views/checks"
	"apparently-typing/internal/views/gameoflife"
	"apparently-typing/internal/views/health"
	"apparently-typing/internal/views/home"
	"net/http"
)

func (s *Server) RegisterRoutes() http.Handler {
	mux := http.NewServeMux()

	// Register routes
	fileServer := http.FileServer(http.FS(Files))
	mux.Handle("/assets/", fileServer)

	health := health.NewHandler()
	home := home.NewHandler()
	checks := checks.NewHandler()
	anim := anim.NewHandler()
	gameoflife := gameoflife.NewHandler()
	mux.Handle("/", home)
	mux.Handle("/healthcheck", health)
	mux.Handle("/checks", checks)
	mux.Handle("/checks/{id}", checks)
	mux.Handle("/anim", anim)
	mux.Handle("/gameoflife", gameoflife)
	// Wrap the mux with CORS middleware
	return s.corsMiddleware(mux)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// w.Header().Set("Access-Control-Allow-Origin", "*") // Replace "*" with specific origins if needed
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")
		w.Header().Set("Access-Control-Allow-Credentials", "false") // Set to "true" if credentials are required

		// Handle preflight OPTIONS requests
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		// Proceed with the next handler
		next.ServeHTTP(w, r)
	})
}
