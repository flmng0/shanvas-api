package main

import "timd.dev/shanvas-api/shanvas"

const Scale = 10
const AspectWidth = 4
const AspectHeight = 3
const StateFilePath = "canvas.bin"

func main() {
	config := shanvas.Config{
		Scale:         Scale,
		AspectWidth:   AspectWidth,
		AspectHeight:  AspectHeight,
		StateFilePath: StateFilePath,
		Port:          5678,
	}
	shanvas.Run(config)
}
