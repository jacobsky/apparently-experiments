package checks

import (
	"log/slog"
	"net/http"

	"github.com/a-h/templ"
)

const X_DIMENSION uint = 20
const Y_DIMENSION uint = 20
const channelBuffer uint = 10

type Message struct {
	id    string
	state bool
}

type Handler struct {
	state [X_DIMENSION][Y_DIMENSION]bool
	tx    chan Message
	rx    []chan Message
	addRx chan chan Message
	delRx chan (<-chan Message)
}

func NewHandler() http.Handler {
	state := [X_DIMENSION][Y_DIMENSION]bool{}
	h := &Handler{
		state: state,
		tx:    make(chan Message, channelBuffer),
		rx:    make([]chan Message, 0),
		addRx: make(chan chan Message, channelBuffer),
		delRx: make(chan (<-chan Message), channelBuffer),
	}
	go h.serve()
	return h
}

func (h *Handler) serve() {
	for {
		select {
		case msg := <-h.tx:
			slog.Debug("Update message received adding to broadcasting")

			for _, rx := range h.rx {
				rx <- msg
			}

		case channel := <-h.addRx:
			slog.Debug("Opening channel", "channel", channel)
			h.rx = append(h.rx, channel)
		case channel := <-h.delRx:
			slog.Debug("Closing channel", "channel", channel)
			for i, ch := range h.rx {
				if ch == channel {
					h.rx[i] = h.rx[len(h.rx)-1]
					h.rx = h.rx[:len(h.rx)-1]
					close(ch)
					break
				}
			}
		}
	}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {

	case http.MethodGet:
		if r.URL.Query().Has("listen") {
			h.listen(w, r)
		} else {
			templ.Handler(Checkboxes(&h.state, X_DIMENSION, Y_DIMENSION)).ServeHTTP(w, r)
		}
	case http.MethodPost:
		h.update(w, r)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not yet implemented", http.StatusNotImplemented)
}

func (h *Handler) listen(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Not yet implemented", http.StatusNotImplemented)
}
