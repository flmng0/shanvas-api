package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
)

type paintEvent struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Brush int `json:"brush"`
}

func NewPaintEvent(x, y, brush int) paintEvent {
	return paintEvent{X: x, Y: y, Brush: brush}
}

func sseMessage(event, data string, id int) string {
	return fmt.Sprintf("event: %v\ndata: %v\nid: %v\n\n", event, data, id)
}

type shanvasApi struct {
	canvas       []byte
	canvasWidth  uint
	canvasHeight uint
	stateFile    string
	listeners    []chan paintEvent
}

func NewShanvasApi(stateFile string, canvasWidth, canvasHeight uint) (*shanvasApi, error) {
	canvasSize := canvasWidth * canvasHeight
	var canvas []byte

	stat, err := os.Stat(stateFile)
	readData := false
	if err == nil && stat.Size() == int64(canvasSize) {
		// File exists. If it has the correct size, use it as the state of the canvas.
		canvas, err = os.ReadFile(stateFile)
		if err != nil {
			return nil, err
		}

		readData = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if !readData {
		canvas = make([]byte, canvasSize)

		err = os.WriteFile(stateFile, canvas, 0644)
		if err != nil {
			return nil, err
		}
	}

	return &shanvasApi{
		canvas:       canvas,
		canvasWidth:  canvasWidth,
		canvasHeight: canvasHeight,
		stateFile:    stateFile,
	}, nil
}

var ErrPaintOutOfBounds = errors.New("paint out of bounds")
var ErrInvalidBrush = errors.New("invalid brush")

func (api *shanvasApi) ApplyPaint(event paintEvent) error {
	if event.X < 0 || uint(event.X) >= api.canvasWidth {
		return ErrPaintOutOfBounds
	}
	if event.Y < 0 || uint(event.Y) >= api.canvasHeight {
		return ErrPaintOutOfBounds
	}
	// TODO: Don't use magic number!
	if event.Brush < 0 || event.Brush > 7 {
		return ErrInvalidBrush
	}

	idx := uint(event.X) + api.canvasWidth*uint(event.Y)
	api.canvas[idx] = byte(event.Brush)

	for _, listener := range api.listeners {
		listener <- event
	}

	return nil
}

func (api *shanvasApi) HandleCanvas(w http.ResponseWriter, r *http.Request) {
	header := w.Header()

	switch r.Method {
	case http.MethodGet:
		header.Add("Content-Type", "application/octet-stream")
		reader := bytes.NewReader(api.canvas)
		io.Copy(w, reader)

	case http.MethodPatch:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var event paintEvent

		if err := json.Unmarshal(body, &event); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		api.ApplyPaint(event)

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *shanvasApi) StreamUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	header := w.Header()
	header.Add("Content-Type", "text/event-stream")
	header.Add("Cache-Control", "no-cache")

	listener := make(chan paintEvent)
	api.listeners = append(api.listeners, listener)

	for event := range listener {
		data, err := json.Marshal(event)
		if err != nil {
			log.Printf("Failed to marshal event: %v\nError: %v\n", event, err)
		}
		message := sseMessage("paint", string(data), 0)
		fmt.Fprint(w, message)

		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

const Scale = 10
const AspectWidth = 4
const AspectHeight = 3

func main() {
	api, err := NewShanvasApi("canvas.bin", Scale*AspectWidth, Scale*AspectHeight)
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", api.HandleCanvas)
	mux.HandleFunc("/sse", api.StreamUpdates)

	addr := "127.0.0.1:5481"
	fmt.Printf("Listening on: %v\n", addr)
	http.ListenAndServe(addr, mux)
}
