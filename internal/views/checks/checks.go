package checks

import (
	"apparently-experiments/internal/shared"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

const X_DIMENSION uint = 20
const Y_DIMENSION uint = 20

const URI_PARAM_LISTEN string = "listen"
const URI_PARAM_ID string = "id"
const URI_PARAM_X string = "x"
const URI_PARAM_Y string = "y"
const URI_PARAM_STATE string = "state"

const channelBuffer uint = 10

type SyncMap struct {
	rw     sync.RWMutex
	values [X_DIMENSION][Y_DIMENSION]bool
}

func NewSyncMap() SyncMap {
	return SyncMap{
		rw:     sync.RWMutex{},
		values: [X_DIMENSION][Y_DIMENSION]bool{},
	}
}

func (sm *SyncMap) Get(x, y uint) bool {
	sm.rw.RLock()
	defer sm.rw.RUnlock()
	return sm.values[x][y]
}
func (sm *SyncMap) Set(x, y uint, value bool) {
	slog.Debug("Update message received adding to broadcasting", "x", x, "y", y, "value", value)
	sm.rw.Lock()
	defer sm.rw.Unlock()
	sm.values[x][y] = value
}

type Message struct {
	X     uint `json:"x"`
	Y     uint `json:"y"`
	Value bool `json:"value"`
}

type Handler struct {
	checkboxes SyncMap
	tx         chan Message
	rx         []chan Message
	addRx      chan chan Message
	delRx      chan (<-chan Message)
}

func NewHandler() http.Handler {
	h := &Handler{
		checkboxes: NewSyncMap(),
		tx:         make(chan Message, channelBuffer),
		rx:         make([]chan Message, 0),
		addRx:      make(chan chan Message, channelBuffer),
		delRx:      make(chan (<-chan Message), channelBuffer),
	}
	go h.serve()
	return h
}

func (h *Handler) serve() {
	slog.Debug("Checks updater worker started")
	for {
		select {
		case msg := <-h.tx:
			slog.Debug("Update message received adding to broadcasting", "x", msg.X, "y", msg.Y, "value", msg.Value)
			h.checkboxes.Set(msg.X, msg.Y, msg.Value)
			for _, rx := range h.rx {
				rx <- msg
			}

		case channel := <-h.addRx:
			slog.Debug("Opening channel")
			h.rx = append(h.rx, channel)

		case channel := <-h.delRx:
			slog.Debug("Closing channel")
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
		if r.URL.Query().Has(URI_PARAM_LISTEN) {
			h.listen(w, r)
		} else {
			templ.Handler(Checkboxes(&h.checkboxes, X_DIMENSION, Y_DIMENSION)).ServeHTTP(w, r)
		}
	case http.MethodPost:
		h.update(w, r)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	slog.Debug("Checkbox update sent")
	sse := datastar.NewSSE(w, r)

	x, err := strconv.Atoi(r.URL.Query().Get(URI_PARAM_X))
	if err != nil {
		_ = sse.ConsoleError(err)
		return
	}

	y, err := strconv.Atoi(r.URL.Query().Get(URI_PARAM_Y))
	if err != nil {
		_ = sse.ConsoleError(err)
		return
	}

	state, err := strconv.ParseBool(r.URL.Query().Get(URI_PARAM_STATE))
	if err != nil {
		_ = sse.ConsoleError(fmt.Errorf("internal error %v", err))
		return
	}
	msg := Message{
		X:     uint(x),
		Y:     uint(y),
		Value: state,
	}
	slog.Debug("message injested", "message", msg)
	h.tx <- msg
}

func (h *Handler) listen(w http.ResponseWriter, r *http.Request) {
	requestId := r.Context().Value(shared.ContextRequestIDHeader)
	slog.Debug("Checkbox listen()", "request_id", requestId)
	sse := datastar.NewSSE(w, r)

	err := sse.PatchElementTempl(CheckboxesFragment(&h.checkboxes, X_DIMENSION, Y_DIMENSION))
	if err != nil {
		_ = sse.ConsoleError(err)
		return
	}
	listener := make(chan Message)
	h.addRx <- listener
	slog.Debug("Checkbox listener connected", "request_id", requestId)
	// Keep the context open until the connection closes (detectable via the request context)
	for {
		select {
		case <-sse.Context().Done():
			slog.Debug("Checkbox listener disconnected", "request_id", requestId)
			h.delRx <- listener
			return
		case msg := <-listener:
			err := sse.PatchElementTempl(Checkbox(msg.X, msg.Y, msg.Value))
			if err != nil {
				slog.Error("Error occurred when patching", "error", err)
			}
		}
	}
}
