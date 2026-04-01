package shanvas

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Config struct {
	StateFilePath string
	Port          int

	CanvasWidth  int
	CanvasHeight int
}

func loadState(path string) ([]byte, error) {
	_, err := os.Stat(path)

	if err == nil {
		data, err := os.ReadFile(path)
		return data, err
	}

	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	return nil, err
}

func saveState(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func autoSave(ctx context.Context, interval time.Duration, canvas *Canvas, path string) {
	timer := time.NewTimer(interval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("Quit detected. Saving!")
			saveState(path, canvas.Bytes())
			return

		case <-timer.C:
			timer.Reset(interval)
			saveState(path, canvas.Bytes())
		}
	}
}

func Run(config Config) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	data, err := loadState(config.StateFilePath)
	if err != nil {
		log.Fatal(err)
	}

	canvas := NewCanvas(data, config.CanvasWidth, config.CanvasHeight)
	saveState(config.StateFilePath, canvas.Bytes())

	api, err := NewApi(ctx, canvas)
	if err != nil {
		log.Fatal(err)
	}

	addr := fmt.Sprintf("127.0.0.1:%v", config.Port)

	server := &http.Server{
		Addr:    addr,
		Handler: api,
	}

	go func() {
		log.Printf("Server listening on: %v\n", server.Addr)

		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()
	go autoSave(ctx, time.Second*10, &canvas, config.StateFilePath)

	<-ctx.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}
