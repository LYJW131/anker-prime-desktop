package cover

import (
	"bytes"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"

	// Decoders registered for image.Decode.
	_ "image/gif"
	_ "image/png"

	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	// Side is the cover edge length in pixels.
	Side = 240
	// Quality is the baseline JPEG quality used for covers.
	Quality = 85
)

// CRC returns the IEEE CRC-32 the charger knows a cover by.
func CRC(jpegData []byte) uint32 { return crc32.ChecksumIEEE(jpegData) }

// EncodeCover center-crops img to a square, resizes it to 240x240 and
// encodes a baseline JPEG. Go's encoder writes 4:2:0 chroma; the device was
// verified with a 4:4:4 JPEG, so test this output on hardware before relying on it.
func EncodeCover(img image.Image) ([]byte, error) {
	sq := CropSquare(img)
	dst := image.NewRGBA(image.Rect(0, 0, Side, Side))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.Black), image.Point{}, draw.Src)
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), sq, sq.Bounds(), xdraw.Over, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: Quality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// CropSquare returns the centered square region of img (a view, no copy).
func CropSquare(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	s := min(w, h)
	x0 := b.Min.X + (w-s)/2
	y0 := b.Min.Y + (h-s)/2
	r := image.Rect(x0, y0, x0+s, y0+s)
	if sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return sub.SubImage(r)
	}
	return &cropped{img, r}
}

type cropped struct {
	image.Image
	r image.Rectangle
}

func (c *cropped) Bounds() image.Rectangle { return c.r }

// DecodeFile decodes png/jpeg/gif/webp/bmp/tiff directly and falls back to
// macOS sips (HEIC and others) through a temporary PNG.
func DecodeFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err == nil {
		return img, nil
	}
	if !errors.Is(err, image.ErrFormat) {
		return nil, err
	}
	return decodeViaSips(path)
}

func decodeViaSips(path string) (image.Image, error) {
	dir, err := os.MkdirTemp("", "cover-sips-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "out.png")
	if msg, err := exec.Command("/usr/bin/sips", "-s", "format", "png", path, "--out", out).CombinedOutput(); err != nil {
		return nil, fmt.Errorf("unsupported image format (sips: %v: %s)", err, bytes.TrimSpace(msg))
	}
	f, err := os.Open(out)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}
