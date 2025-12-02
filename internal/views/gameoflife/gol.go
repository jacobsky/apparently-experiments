package gameoflife

import (
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/a-h/templ"
	"github.com/starfederation/datastar-go/datastar"
)

const tickTime = 2000
const channelBuffer = 10
const boardSizeX = 50
const boardSizeY = 50

type TileUpdate struct {
	X     uint
	Y     uint
	Value bool
}

type GameBoard struct {
	rw    sync.RWMutex
	board [boardSizeX][boardSizeY]bool
}

func (gb *GameBoard) SetBoard(board [boardSizeX][boardSizeY]bool) {
	gb.rw.Lock()
	defer gb.rw.Unlock()
	gb.board = board
}

func (gb *GameBoard) GetTile(x, y uint) (bool, error) {
	if x >= boardSizeX || y >= boardSizeY {
		return false, fmt.Errorf("coordinate (%v, %v) is greater than the bounds of the board (%v, %v)", x, y, boardSizeX, boardSizeY)
	}
	gb.rw.RLock()
	defer gb.rw.RUnlock()
	return gb.board[x][y], nil
}

func (gb *GameBoard) SetTile(x, y uint, value bool) error {
	if x >= boardSizeX || y >= boardSizeY {
		return fmt.Errorf("coordinate (%v, %v) is greater than the bounds of the board (%v, %v)", x, y, boardSizeX, boardSizeY)
	}

	gb.rw.Lock()
	defer gb.rw.Unlock()
	gb.board[x][y] = value
	return nil
}

func NewGameBoard() GameBoard {
	return GameBoard{
		rw:    sync.RWMutex{},
		board: [boardSizeX][boardSizeY]bool{},
	}
}

// Creates a board with a semi^randomized starting position
func NewRandomGameBoard() GameBoard {
	board := [boardSizeX][boardSizeY]bool{}

	for y := range boardSizeY {
		for x := range boardSizeX {
			if rand.Intn(2) == 0 {
				board[x][y] = true
			}
		}
	}
	return GameBoard{
		rw:    sync.RWMutex{},
		board: board,
	}
}

type Handler struct {
	rw    sync.RWMutex
	tx    chan *TileUpdate
	rx    []chan *GameBoard
	addRx chan chan *GameBoard
	delRx chan (<-chan *GameBoard)
	board GameBoard
}

func NewHandler() http.Handler {
	h := &Handler{
		tx:    make(chan *TileUpdate, channelBuffer),
		rx:    make([]chan *GameBoard, 0),
		addRx: make(chan chan *GameBoard, channelBuffer),
		delRx: make(chan (<-chan *GameBoard), channelBuffer),
		board: NewRandomGameBoard(),
	}
	go h.serve()
	return h
}

func (h *Handler) tickGame() {
	// Create the next frame
	newBoard := [boardSizeX][boardSizeY]bool{}
	h.rw.RLock()

	for x := range boardSizeX {
		for y := range boardSizeY {
			// Calculate whether the cell should be alive or dead as per Conway's game of life rules
			numNeighbors := 0

			// Left neighbors
			if x > 0 {
				if y > 0 && h.board.board[x-1][y-1] {
					numNeighbors++
				}
				if h.board.board[x-1][y] {
					numNeighbors++
				}
				if y < boardSizeY-1 && h.board.board[x-1][y+1] {
					numNeighbors++
				}
			}

			// Middle neighbors
			if y > 0 && h.board.board[x][y-1] {
				numNeighbors++
			}
			if h.board.board[x][y] {
				numNeighbors++
			}
			if y < boardSizeY-1 && h.board.board[x][y+1] {
				numNeighbors++
			}

			// Right neighbors
			if x < boardSizeX-1 {
				if y > 0 && h.board.board[x+1][y-1] {
					numNeighbors++
				}
				if h.board.board[x+1][y] {
					numNeighbors++
				}
				if y < boardSizeY-1 && h.board.board[x+1][y+1] {
					numNeighbors++
				}
			}

			// As per Game of life rules, a cell is living (true) if there is either 2 or three neighbors
			// In all other quanitites, it dies (false)
			if (newBoard[x][y] && numNeighbors == 2) || numNeighbors == 3 {
				newBoard[x][y] = true
			} else {
				newBoard[x][y] = false
			}
		}
	}
	h.rw.RUnlock()

	h.board.SetBoard(newBoard)
}

func (h *Handler) serve() {
	slog.Info("Updater worker started")
	ticker := time.NewTicker(tickTime * time.Millisecond)

	for {
		select {
		case update := <-h.tx:
			err := h.board.SetTile(update.X, update.Y, update.Value)
			if err != nil {
				slog.Error("update tile error", "error", err)
			}

		case <-ticker.C:
			slog.Info("game update")
			h.tickGame()

			h.rw.RLock()

			for _, rx := range h.rx {
				rx <- &h.board
			}
			h.rw.RUnlock()

		case channel := <-h.addRx:
			slog.Info("Opening channel")
			h.rx = append(h.rx, channel)

		case channel := <-h.delRx:
			slog.Info("Closing channel")
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
	case http.MethodPost:
		h.fliptile(w, r)

	case http.MethodGet:
		if r.URL.Query().Has("listen") {
			h.listen(w, r)
		} else {
			h.rw.RLock()
			defer h.rw.RUnlock()
			templ.Handler(GameOfLife(&h.board)).ServeHTTP(w, r)
		}

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}

}

func (h *Handler) listen(w http.ResponseWriter, r *http.Request) {
	slog.Info("game of lifelisten()")
	sse := datastar.NewSSE(w, r)

	err := sse.PatchElementTempl(GameOfLifeFragment(&h.board))
	if err != nil {
		_ = sse.ConsoleError(err)
		return
	}
	listener := make(chan *GameBoard)
	h.addRx <- listener
	slog.Info("game of lifelistener connected")
	// Keep the context open until the connection closes (detectable via the request context)
	for {
		select {
		case <-sse.Context().Done():
			slog.Info("game of lifelistener disconnected")
			h.delRx <- listener
			return
		case msg := <-listener:
			slog.Info("Update sending")
			err := sse.PatchElementTempl(GameOfLifeFragment(msg))
			if err != nil {
				slog.Error("Error occurred when patching", "error", err)
			}
		}
	}
}

func (h *Handler) fliptile(w http.ResponseWriter, r *http.Request) {
	slog.Info("fliptile")
	sse := datastar.NewSSE(w, r)
	id := r.URL.Query().Get("id")

	xcomponent, ycomponent, found := strings.Cut(id, "-")
	if !found {
		_ = sse.ConsoleError(fmt.Errorf("%v, %v was malformed", xcomponent, ycomponent))
		return
	}

	x, err := strconv.Atoi(xcomponent)
	if err != nil {
		_ = sse.ConsoleError(err)
		return
	}

	y, err := strconv.Atoi(ycomponent)
	if err != nil {
		_ = sse.ConsoleError(err)
		return
	}

	isAlive, err := h.board.GetTile(uint(x), uint(y))
	if err != nil {
		_ = sse.ConsoleError(err)
		return
	}
	h.tx <- &TileUpdate{
		X: uint(x), Y: uint(y), Value: !isAlive,
	}

	err = sse.PatchElementTempl(Cell(id, !isAlive))
	if err != nil {
		_ = sse.ConsoleError(err)
		return
	}
}
