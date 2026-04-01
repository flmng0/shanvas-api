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
	"sync/atomic"
)

type paintEvent struct {
	X     int `json:"x"`
	Y     int `json:"y"`
	Brush int `json:"brush"`

	id uint64
}

func NewPaintEvent(x, y, brush int, id uint64) paintEvent {
	return paintEvent{X: x, Y: y, Brush: brush, id: id}
}

func sseMessage(event, data string, id uint64) string {
	return fmt.Sprintf("event: %v\ndata: %v\nid: %v\n\n", event, data, id)
}

type Api struct {
	canvas       Canvas
	tokenHandler *TokenHandler
	apiSecretKey string
	ctx          context.Context
	mux          *http.ServeMux

	eventId   atomic.Uint64
	listeners map[string]chan paintEvent
}

func NewApi(ctx context.Context, canvas Canvas) (*Api, error) {
	var api Api

	api.canvas = canvas
	api.ctx = ctx
	api.listeners = make(map[string]chan paintEvent)

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
	api.mux.HandleFunc("/authorize", api.HandleToken)
	api.mux.HandleFunc("/config", api.HandleConfig)
	api.mux.HandleFunc("/", api.HandleCanvas)

	return &api, nil
}

func getToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")

	if token, found := strings.CutPrefix(authHeader, "Bearer "); found {
		return token
	}

	tokenCookie, err := r.Cookie("_shanvas_token")
	if err == nil {
		return tokenCookie.Value
	}

	return ""
}

func (api *Api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := api.ctx

	if r.URL.Path != "/authorize" {
		token := getToken(r)
		if token == "" {
			http.Error(w, "No required authorization", http.StatusForbidden)
			return
		}

		uid, err := api.tokenHandler.Verify(token)
		if err != nil {
			http.Error(w, "Token invalid!", http.StatusForbidden)
			return
		}

		ctx = context.WithValue(ctx, "uid", uid)
	}

	r = r.WithContext(ctx)

	api.mux.ServeHTTP(w, r)
}

var ErrInvalidBrush = errors.New("invalid brush")

func (api *Api) ApplyPaint(event paintEvent, uid string) error {
	if event.Brush > 7 {
		return ErrInvalidBrush
	}

	err := api.canvas.Paint(byte(event.Brush), event.X, event.Y)
	if err != nil {
		return err
	}

	api.eventId.Add(1)

	for lid, listener := range api.listeners {
		if uid == lid {
			continue
		}
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

		uid, ok := r.Context().Value("uid").(string)
		if !ok {
			http.Error(w, "No UID for request", http.StatusExpectationFailed)
			return
		}

		var event paintEvent

		if err := json.Unmarshal(body, &event); err != nil {
			message := fmt.Sprintf("Bad format: %v", err.Error())
			http.Error(w, message, http.StatusBadRequest)
			return
		}

		event.id = api.eventId.Load()
		err = api.ApplyPaint(event, uid)
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
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST accepted", http.StatusMethodNotAllowed)
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

	cookie := http.Cookie{
		Name:  "_shanvas_token",
		Value: api.tokenHandler.Generate(),
	}
	http.SetCookie(w, &cookie)
}

func (api *Api) StreamUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Only GET accepted", http.StatusMethodNotAllowed)
		return
	}

	uid, ok := r.Context().Value("uid").(string)
	if !ok {
		http.Error(w, "No UID for request", http.StatusExpectationFailed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Failed to create flusher for response writer", http.StatusInternalServerError)
		return
	}

	header := w.Header()
	header.Add("Content-Type", "text/event-stream")
	header.Add("Cache-Control", "no-cache")

	message := sseMessage("connect", "", 0)
	fmt.Fprint(w, message)
	flusher.Flush()

	listener := make(chan paintEvent)
	api.listeners[uid] = listener

	for {
		select {
		case <-r.Context().Done():
			message := sseMessage("cease", "", 0)
			fmt.Fprint(w, message)
			flusher.Flush()

			delete(api.listeners, uid)

			return

		case event := <-listener:
			data, err := json.Marshal(event)
			if err != nil {
				log.Printf("Failed to marshal event: %v\nError: %v\n", event, err)
				continue
			}

			message := sseMessage("paint", string(data), event.id)
			fmt.Fprint(w, message)
			flusher.Flush()
		}
	}
}
