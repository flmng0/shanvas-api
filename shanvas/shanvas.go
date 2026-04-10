package shanvas

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type Config struct {
	StateFilePath    string
	Port             int
	AutoSaveInterval string

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
	if len(data) == 0 {
		log.Println("Tried to save 0 bytes of data!")
		return nil
	}

	file, err := os.OpenFile(path, os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	reader := bytes.NewReader(data)
	n, err := io.Copy(file, reader)
	if err != nil {
		return err
	}

	if n != int64(len(data)) {
		log.Printf("Failed to write all data. Wrote: %v\n", n)
	}

	return nil
}

func autoSave(ctx context.Context, interval time.Duration, canvas *Canvas, path string) {
	timer := time.NewTicker(interval)

	for {
		select {
		case <-ctx.Done():
			log.Println("Quit detected. Saving!")
			if err := saveState(path, canvas.Bytes()); err != nil {
				log.Fatal(err)
			}
			return

		case <-timer.C:
			log.Println("Autosaving...")
			saveState(path, canvas.Bytes())
		}
	}
}

func Run(config Config) {
	autoSaveInterval, err := time.ParseDuration(config.AutoSaveInterval)
	if err != nil {
		autoSaveInterval = 10 * time.Minute
		log.Printf("Failed to parse AutoSaveInterval, using default (10 minutes): %v\n", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	data, err := loadState(config.StateFilePath)
	if err != nil {
		log.Fatal(err)
	}
	if data == nil {
		log.Println("Warning! No initial data could be loaded!")
	}

	canvas := NewCanvas(data, config.CanvasWidth, config.CanvasHeight)
	err = saveState(config.StateFilePath, canvas.Bytes())
	if err != nil {
		log.Fatal(err)
	}

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
	go autoSave(ctx, autoSaveInterval, canvas, config.StateFilePath)

	<-ctx.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal(err)
	}
}
