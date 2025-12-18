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
		if r.Header.Get(shared.RequestIDHeader) == "" {
			requestId := uuid.New().String()
			r.Header.Set(shared.RequestIDHeader, requestId)
			ctx := context.WithValue(r.Context(), shared.ContextRequestIDHeader, requestId)
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

// This is used to get access to the status code and headers returned by the writer
type loggingResponseWriter struct {
	http.ResponseWriter
	status int
}

func wrapResponseWriter(w http.ResponseWriter) *loggingResponseWriter {
	return &loggingResponseWriter{
		ResponseWriter: w,
		status:         http.StatusOK,
	}
}

func (rw *loggingResponseWriter) Status() int {
	return rw.status
}

func (rw *loggingResponseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

// Logs the request lifecycle as well as the roundtrip time
// For each request we need to add in some basic prometheus and logging metrics.
func (s *Server) observabilityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestStart := time.Now()
		totalRequests.Inc()
		concurrentRequests.Inc()
		requestType := "http request"
		responseCode := http.StatusOK

		// Event streams with datastar will hang if the responseWriter is wrapped.
		if r.Header.Get("Datastar-Request") == "true" {
			requestType = "datastar request"
			next.ServeHTTP(w, r)
			responseCode = http.StatusNoContent
		} else {
			responseWriter := wrapResponseWriter(w)
			next.ServeHTTP(responseWriter, r)
		}

		slog.Info(requestType,
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
			time.Since(requestStart),
		)
		concurrentRequests.Dec()
	})
}
