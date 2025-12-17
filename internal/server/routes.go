package server

import (
	"apparently-experiments/internal/shared"
	"apparently-experiments/internal/views/anim"
	"apparently-experiments/internal/views/checks"
	"apparently-experiments/internal/views/clock"
	"apparently-experiments/internal/views/gameoflife"
	"apparently-experiments/internal/views/health"
	"apparently-experiments/internal/views/home"
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var concurrentRequests = promauto.NewGauge(prometheus.GaugeOpts{
	Name: "concurrentRequests",
	Help: "Number of requests actively in flight",
})

var totalRequests = promauto.NewCounter(prometheus.CounterOpts{
	Name: "totalRequests",
	Help: "The total number of processed requests",
})

func (s *Server) RegisterRoutes() http.Handler {
	mux := http.NewServeMux()

	// Register routes
	fileServer := http.FileServer(http.FS(Files))
	mux.Handle("/assets/", fileServer)

	health := health.NewHandler()
	home := home.NewHandler()
	checks := checks.NewHandler()
	clock := clock.NewHandler()
	anim := anim.NewHandler()
	gameoflife := gameoflife.NewHandler()

	mux.Handle("/", home)
	mux.Handle("/healthcheck", health)
	mux.Handle("/checks", checks)
	mux.Handle("/checks/{id}", checks)
	mux.Handle("/clock", clock)
	mux.Handle("/anim", anim)
	mux.Handle("/gameoflife", gameoflife)
	mux.Handle("/metrics", promhttp.Handler())
	// Wrap the mux with CORS middleware
	return s.addRequestHeaderMiddleware(s.observabilityMiddleware(s.corsMiddleware(mux)))
}

// This middleware is used to add a unique request ID which can be used to help trace details.
func (s *Server) addRequestHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(shared.RequestIDHeader) == "" {
			requestId := uuid.New().String()
			r.Header.Set(shared.RequestIDHeader, requestId)
			ctx := context.WithValue(r.Context(), shared.RequestIDHeader, requestId)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
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

// Logs the request lifecycle as well as the roundtrip time
// For each request we need to add in some basic prometheus and logging metrics.
func (s *Server) observabilityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestStart := time.Now()
		totalRequests.Inc()
		concurrentRequests.Inc()

		next.ServeHTTP(w, r)

		concurrentRequests.Dec()
		slog.Info("request", "time", requestStart, "request_id", r.Context().Value(shared.RequestIDHeader), "url", r.URL.Path, "duration", time.Since(requestStart))
	})
}
