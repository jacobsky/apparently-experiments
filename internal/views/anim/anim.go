package anim

import (
	"apparently-experiments/internal/shared"
	"log/slog"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

const (
	channelBuffer  = 10
	ticksPerSecond = 30
)

type AnimationState struct {
	tick  float64
	red   uint8
	green uint8
	blue  uint8
	x_pos int
	y_pos int
}
type Handler struct {
	rw    sync.RWMutex
	rx    []chan *AnimationState
	addRx chan chan *AnimationState
	delRx chan (<-chan *AnimationState)
	anim  AnimationState
}

func NewHandler() http.Handler {
	h := &Handler{
		rw:    sync.RWMutex{},
		rx:    make([]chan *AnimationState, 0),
		addRx: make(chan chan *AnimationState, channelBuffer),
		delRx: make(chan (<-chan *AnimationState), channelBuffer),
		anim: AnimationState{
			tick:  0,
			red:   255,
			green: 0,
			blue:  0,
			x_pos: 0,
			y_pos: 0,
		},
	}
	go h.serve()
	return h
}

func (h *Handler) tickAnimation() {
	// Will spin at a rate of 360 per 5 seconds aka 2pi per 5 seconds, so 2pi / 5
	const rateOfChange = 2 * math.Pi / 5

	h.rw.Lock()
	defer h.rw.Unlock()
	h.anim.tick += rateOfChange / ticksPerSecond
	// Changing colors
	h.anim.red = uint8(math.Cos(float64(h.anim.tick)) * 255)
	h.anim.green = uint8(math.Cos(float64(h.anim.tick)+math.Pi) * 255)
	h.anim.blue = uint8((math.Cos(float64(h.anim.tick) + math.Pi + math.Pi)) * 255)
	// Circular animation
	h.anim.x_pos = int(math.Cos(float64(h.anim.tick))*50 + 100)
	h.anim.y_pos = int(math.Sin(float64(h.anim.tick))*50 + 100)

}

func (h *Handler) serve() {
	slog.Info("Animation handler update worker start")
	ticker := time.NewTicker(time.Second / ticksPerSecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Pause the animation if no one is watching
			// Clean up the ticker during this time.
			if len(h.rx) == 0 {
				ticker.Stop()
				continue
			}
			h.tickAnimation()

			h.rw.RLock()

			for _, rx := range h.rx {
				rx <- &h.anim
			}
			h.rw.RUnlock()

		case channel := <-h.addRx:
			slog.Debug("Opening channel")
			// If this is the first viewer, start the animation.
			if len(h.rx) == 0 {
				ticker.Reset(time.Second / ticksPerSecond)
			}
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
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.URL.Query().Has("listen") {
		h.listen(w, r)
		return
	}
	h.rw.RLock()
	defer h.rw.RUnlock()
	templ.Handler(Animation(&h.anim)).ServeHTTP(w, r)
}

func (h *Handler) listen(w http.ResponseWriter, r *http.Request) {
	requestId := r.Context().Value(shared.ContextRequestIDHeader)
	slog.Debug("Animation listen()", "request_id", requestId)
	sse := datastar.NewSSE(w, r)

	err := sse.PatchElementTempl(AnimationFragment(&h.anim))
	if err != nil {
		_ = sse.ConsoleError(err)
		return
	}
	listener := make(chan *AnimationState)
	h.addRx <- listener
	slog.Debug("Animation listener connected", "request_id", requestId)
	// Keep the context open until the connection closes (detectable via the request context)
	for {
		select {

		case <-sse.Context().Done():
			slog.Debug("Animation listener disconnected", "request_id", requestId)
			h.delRx <- listener
			return

		case msg := <-listener:
			err := sse.PatchElementTempl(AnimationFragment(msg))
			if err != nil {
				slog.Error("Error occurred when patching", "error", err, "request_id", requestId)
			}
		}
	}
}
