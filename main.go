package main

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"timd.dev/shanvas-api/shanvas"
)

const Scale = 10
const AspectWidth = 4
const AspectHeight = 3
const StateFilePath = "canvas.bin"

func main() {
	isDev := os.Getenv("SHANVAS_DEV") != ""

	if isDev {
		err := godotenv.Load(".env.local")

		if err != nil {
			log.Printf("Failed to load dotenv: %v\n", err)
		}
	}

	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		port = 5678
	}

	config := shanvas.Config{
		Scale:         Scale,
		AspectWidth:   AspectWidth,
		AspectHeight:  AspectHeight,
		StateFilePath: StateFilePath,
		Port:          port,
	}
	shanvas.Run(config)
}
