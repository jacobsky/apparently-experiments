package home

import (
	"net/http"

	"github.com/a-h/templ"
)

type Handler struct{}

func NewHandler() http.Handler {
	return &Handler{}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	templ.Handler(Home()).ServeHTTP(w, r)
}
