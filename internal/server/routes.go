package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"apparently-experiments/internal/shared"
	"apparently-experiments/internal/views/anim"
	"apparently-experiments/internal/views/checks"
	"apparently-experiments/internal/views/clock"
	"apparently-experiments/internal/views/gameoflife"
	"apparently-experiments/internal/views/health"
	"apparently-experiments/internal/views/home"

	"github.com/felixge/httpsnoop"
	"github.com/google/uuid"
	"github.com/justinas/alice"
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
	middleware := alice.New(s.addRequestHeaderMiddleware, s.observabilityMiddleware, s.corsMiddleware)
	mux := http.NewServeMux()
	// Register routes
	health := health.NewHandler()

	fileServer := http.FileServer(http.FS(Files))
	mux.Handle("/assets/", fileServer)
	mux.Handle("/healthcheck", health)
	mux.Handle("/metrics", promhttp.Handler())

	home := home.NewHandler()
	checks := checks.NewHandler()
	clock := clock.NewHandler()
	anim := anim.NewHandler()
	gameoflife := gameoflife.NewHandler()

	mux.Handle("/", middleware.Then(home))
	mux.Handle("/checks", middleware.Then(checks))
	mux.Handle("/checks/{id}", middleware.Then(checks))
	mux.Handle("/clock", middleware.Then(clock))
	mux.Handle("/anim", middleware.Then(anim))
	mux.Handle("/gameoflife", middleware.Then(gameoflife))
	// Wrap the mux with CORS middleware
	return mux
}

// This middleware is used to add a unique request ID which can be used to help trace details.
func (s *Server) addRequestHeaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(shared.RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
			r.Header.Set(shared.RequestIDHeader, requestID)
		}
		// Always set the context, even if header already exists
		ctx := context.WithValue(r.Context(), shared.ContextRequestIDHeader, requestID)
		r = r.WithContext(ctx)
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
// TODO: Split this into a metrics middleware and a log middleware
func (s *Server) observabilityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestStart := time.Now()
		totalRequests.Inc()
		concurrentRequests.Inc()
		requestType := "http request"
		responseCode := http.StatusOK
		m := httpsnoop.CaptureMetrics(next, w, r)

		// Event streams with datastar will hang if the responseWriter is wrapped.
		if r.Header.Get("Datastar-Request") == "true" {
			requestType = "datastar request"
		}
		logFields := []any{
			"time",
			requestStart,
			"request_id",
			r.Context().Value(shared.ContextRequestIDHeader),
			"url",
			r.URL.Path,
			"method",
			r.Method,
			"status",
			responseCode,
			"duration",
			m.Duration,
			"responseSize",
			m.Written,
		}

		// Ensure that it is logged as a proper error if it is internal. This is a definite bug.
		if responseCode >= 500 {
			slog.Error(requestType, logFields...)
		} else if responseCode >= 400 {
			slog.Warn(requestType, logFields...)
		} else {
			slog.Info(requestType, logFields...)
		}
		concurrentRequests.Dec()
	})
}
