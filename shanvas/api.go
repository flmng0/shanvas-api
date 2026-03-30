package shanvas

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
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

type Api struct {
	canvas    Canvas
	ctx       context.Context
	mux       *http.ServeMux
	listeners []chan paintEvent
}

func NewApi(ctx context.Context, canvas Canvas) (*Api, error) {
	var api Api

	api.canvas = canvas
	api.ctx = ctx

	api.mux = http.NewServeMux()

	api.mux.HandleFunc("/", api.HandleCanvas)
	api.mux.HandleFunc("/sse", api.StreamUpdates)

	return &api, nil
}

func (api *Api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = r.WithContext(api.ctx)
	api.mux.ServeHTTP(w, r)
}

var ErrInvalidBrush = errors.New("invalid brush")

func (api *Api) ApplyPaint(event paintEvent) error {
	if event.Brush > 7 {
		return ErrInvalidBrush
	}

	err := api.canvas.Paint(byte(event.Brush), event.X, event.Y)
	if err != nil {
		return err
	}

	for _, listener := range api.listeners {
		listener <- event
	}

	return nil
}

func (api *Api) HandleCanvas(w http.ResponseWriter, r *http.Request) {
	header := w.Header()

	switch r.Method {
	case http.MethodGet:
		header.Add("Content-Type", "application/octet-stream")
		reader := bytes.NewReader(api.canvas.Bytes())
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

		err = api.ApplyPaint(event)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *Api) StreamUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	header := w.Header()
	header.Add("Content-Type", "text/event-stream")
	header.Add("Cache-Control", "no-cache")

	listener := make(chan paintEvent)
	api.listeners = append(api.listeners, listener)

	flusher, ok := w.(http.Flusher)
	if !ok {
		return
	}

	for {
		select {
		case <-r.Context().Done():
			message := sseMessage("cease", "", 0)
			fmt.Fprint(w, message)
			flusher.Flush()

			return

		case event := <-listener:
			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("Failed to marshal event: %v\nError: %v\n", event, err)
			}

			message := sseMessage("paint", string(data), 0)
			fmt.Fprint(w, message)
			flusher.Flush()
		}
	}
}
