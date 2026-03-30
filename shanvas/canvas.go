package shanvas

import (
	"errors"
)

type Canvas struct {
	buffer []byte
	width  int
	height int
}

func NewCanvas(data []byte, width, height int) Canvas {
	if data == nil {
		data = make([]byte, width*height)
	}

	return Canvas{
		buffer: data,
		width:  width,
		height: height,
	}
}

var ErrPaintOutOfBounds = errors.New("tried to paint out of bounds!")

func (cvs Canvas) Paint(value byte, x, y int) error {
	index := x + y*cvs.width
	if index > len(cvs.buffer) {
		return ErrPaintOutOfBounds
	}

	cvs.buffer[index] = value
	return nil
}

func (cvs Canvas) Bytes() []byte {
	return cvs.buffer
}
