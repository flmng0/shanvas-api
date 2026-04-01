package main

import (
	"os"
	"strconv"

	"timd.dev/shanvas-api/shanvas"
)

const StateFilePath = "canvas.bin"
const CanvasWidth = 1000
const CanvasHeight = 1000

func main() {
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		port = 5678
	}

	config := shanvas.Config{
		CanvasWidth:   CanvasWidth,
		CanvasHeight:  CanvasHeight,
		StateFilePath: StateFilePath,
		Port:          port,
	}
	shanvas.Run(config)
}
