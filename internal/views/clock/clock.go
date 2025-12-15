package clock

import (
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

const (
	URI_PARAM_LISTEN   = "listen"
	TICKER_DURATION_MS = 100
)

type Handler struct{}

func NewHandler() http.Handler {
	return &Handler{}
}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {

	case http.MethodGet:
		if r.URL.Query().Has(URI_PARAM_LISTEN) {
			ticker := time.NewTicker(TICKER_DURATION_MS * time.Millisecond)
			ticks := 1
			sse := datastar.NewSSE(w, r)
			for {
				select {
				case <-ticker.C:
					err := sse.PatchElementTempl(ClockFragment(ticks))
					if err != nil {
						_ = sse.ConsoleError(err)
					}

					err = sse.PatchElementTempl(ClockTitle(), datastar.WithSelector("title"))
					if err != nil {
						_ = sse.ConsoleError(err)
					}
					ticks++
				case <-sse.Context().Done():
					return
				}
			}
		} else {
			templ.Handler(Clock()).ServeHTTP(w, r)
		}
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}
