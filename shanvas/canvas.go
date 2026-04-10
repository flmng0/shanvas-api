package shanvas

import (
	"errors"
	"image"
	"image/color"
	"log"
	"strconv"
	"strings"
)

type Canvas struct {
	buffer []byte
	Width  int
	Height int
}

func NewCanvas(initialData []byte, width, height int) *Canvas {
	data := make([]byte, width*height)
	if initialData == nil || len(initialData) != width*height {
		log.Println("Initial data empty, or the wrong size. Using zeros")
	} else {
		copy(data, initialData)
	}

	return &Canvas{
		buffer: data,
		Width:  width,
		Height: height,
	}
}

func (cvs *Canvas) ToImage(palette []string) (image.Image, error) {
	rect := image.Rect(0, 0, cvs.Width, cvs.Height)
	imagePalette := make([]color.Color, len(palette))

	for i, colorText := range palette {
		colorText = strings.TrimPrefix(colorText, "#")
		colorNum, err := strconv.ParseUint(colorText, 16, 24)
		if err != nil {
			return nil, err
		}

		imagePalette[i] = color.NRGBA{
			R: byte(colorNum & 0xff0000 >> 16),
			G: byte(colorNum & 0x00ff00 >> 8),
			B: byte(colorNum & 0x0000ff >> 0),
			A: 0xff,
		}
	}

	image := image.NewPaletted(rect, imagePalette)

	for y := range cvs.Height {
		for x := range cvs.Width {
			pixelIdx := x + y*cvs.Width
			brushIdx := cvs.buffer[pixelIdx]
			image.SetColorIndex(x, y, brushIdx)
		}
	}

	return image, nil
}

var ErrPaintOutOfBounds = errors.New("tried to paint out of bounds!")

func (cvs *Canvas) Paint(value byte, x, y int) error {
	index := x + y*cvs.Width
	if index > len(cvs.buffer) {
		return ErrPaintOutOfBounds
	}

	cvs.buffer[index] = value
	return nil
}

func (cvs *Canvas) Bytes() []byte {
	return cvs.buffer
}
