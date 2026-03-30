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
	"os"
	"strings"
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
	canvas       Canvas
	tokenHandler *TokenHandler
	apiSecretKey string
	ctx          context.Context
	mux          *http.ServeMux
	listeners    []chan paintEvent
}

func NewApi(ctx context.Context, canvas Canvas) (*Api, error) {
	var api Api

	api.canvas = canvas
	api.ctx = ctx

	sessionSecret := os.Getenv("TOKEN_SALT")
	if sessionSecret == "" {
		log.Fatal("TOKEN_SALT not found")
	}

	api.tokenHandler = NewTokenHandler(sessionSecret)
	api.apiSecretKey = os.Getenv("API_SECRET_KEY")
	if api.apiSecretKey == "" {
		log.Fatal("API_SECRET_KEY not found")
	}

	api.mux = http.NewServeMux()

	api.mux.HandleFunc("/sse", api.StreamUpdates)
	api.mux.HandleFunc("/token", api.HandleToken)
	api.mux.HandleFunc("/config", api.HandleConfig)
	api.mux.HandleFunc("/", api.HandleCanvas)

	return &api, nil
}

func (api *Api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/token" {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "No authorization header", http.StatusForbidden)
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Invalid authorization header", http.StatusForbidden)
			return
		}

		if err := api.tokenHandler.Verify(parts[1]); err != nil {
			fmt.Println(err)
			http.Error(w, "Token invalid!", http.StatusForbidden)
			return
		}
	}

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
			http.Error(w, "Unable to read request body", http.StatusBadRequest)
			return
		}

		var event paintEvent

		if err := json.Unmarshal(body, &event); err != nil {
			message := fmt.Sprintf("Bad format: %v", err.Error())
			http.Error(w, message, http.StatusBadRequest)
			return
		}

		err = api.ApplyPaint(event)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

	default:
		http.Error(w, "Expected GET or PATCH", http.StatusMethodNotAllowed)
	}
}

type PublicConfig struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

func (api *Api) HandleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET accepted", http.StatusMethodNotAllowed)
		return
	}

	enc := json.NewEncoder(w)
	enc.Encode(PublicConfig{
		Width:  api.canvas.width,
		Height: api.canvas.height,
	})
}

func (api *Api) HandleToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET accepted", http.StatusMethodNotAllowed)
		return
	}

	auth := r.Header.Get("Authorization")
	if auth == "" {
		http.Error(w, "Missing authorization header", http.StatusForbidden)
		return
	}

	parts := strings.SplitN(auth, " ", 2)
	if parts[0] != "Secret" || parts[1] != api.apiSecretKey {
		http.Error(w, "Secret invalid", http.StatusForbidden)
		return
	}

	token := api.tokenHandler.Generate()
	fmt.Fprint(w, token)
}

func (api *Api) StreamUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET accepted", http.StatusMethodNotAllowed)
		return
	}

	header := w.Header()
	header.Add("Content-Type", "text/event-stream")
	header.Add("Cache-Control", "no-cache")

	listener := make(chan paintEvent)
	api.listeners = append(api.listeners, listener)

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Failed to create flusher for response writer", http.StatusInternalServerError)
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
				continue
			}

			message := sseMessage("paint", string(data), 0)
			fmt.Fprint(w, message)
			flusher.Flush()
		}
	}
}
