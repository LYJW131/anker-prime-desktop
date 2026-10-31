package demo

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	_ "image/gif" // pictures lyjw may add
	"image/jpeg"
	_ "image/png"
	"math"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

// coverSize is the side of the charger's screen in pixels.
const coverSize = 240

// art is one of the demo library's pictures, drawn pixel by pixel.
type art struct {
	name string
	px   func(x, y float64) color.RGBA // x, y in 0…1
}

func (a art) file() string { return a.name + ".jpg" }

func coverFile(id int) string { return fmt.Sprintf("%d.jpg", id) }

func (a art) jpeg() []byte {
	img := image.NewRGBA(image.Rect(0, 0, coverSize, coverSize))
	for y := 0; y < coverSize; y++ {
		for x := 0; x < coverSize; x++ {
			img.SetRGBA(x, y, a.px((float64(x)+0.5)/coverSize, (float64(y)+0.5)/coverSize))
		}
	}
	return encode(img)
}

func encode(img image.Image) []byte {
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}) // encoding to memory cannot fail
	return buf.Bytes()
}

func crc(data []byte) uint32 { return crc32.ChecksumIEEE(data) }

func mix(a, b color.RGBA, t float64) color.RGBA {
	t = math.Min(math.Max(t, 0), 1)
	l := func(x, y uint8) uint8 { return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t)) }
	return color.RGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

func hex(v uint32) color.RGBA { return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255} }

// smooth is a soft step of width w around edge.
func smooth(edge, w, v float64) float64 { return math.Min(math.Max((v-edge)/w+0.5, 0), 1) }

var arts = []art{
	{"晚霞", func(x, y float64) color.RGBA {
		sky := mix(hex(0xff9a5a), hex(0x5b2a86), y*1.1)
		d := math.Hypot(x-0.5, y-0.58)
		sky = mix(sky, hex(0xffe7a8), 1-smooth(0.17, 0.01, d))
		if y > 0.62 {
			sea := mix(hex(0x3a1f5c), hex(0x120a24), (y-0.62)/0.38)
			band := 0.5 + 0.5*math.Sin(y*90)
			if math.Abs(x-0.5) < 0.2*(1-(y-0.62)*1.5) && band > 0.6 {
				sea = mix(sea, hex(0xffc98a), 0.55)
			}
			return sea
		}
		return sky
	}},
	{"极光", func(x, y float64) color.RGBA {
		c := mix(hex(0x07122b), hex(0x0b1f3a), y)
		for i, col := range []uint32{0x3cf2b0, 0x4fd2ff, 0x9b7bff} {
			fi := float64(i)
			mid := 0.32 + 0.12*fi + 0.07*math.Sin(x*6+fi*1.7)
			g := math.Exp(-math.Pow((y-mid)/0.05, 2))
			c = mix(c, hex(col), g*0.75)
		}
		if math.Mod(x*97+y*61, 1) < 0.008 && y < 0.5 {
			c = hex(0xffffff)
		}
		return c
	}},
	{"同心", func(x, y float64) color.RGBA {
		d := math.Max(math.Abs(x-0.5), math.Abs(y-0.5))
		ring := int(d * 14)
		if ring%2 == 0 {
			return mix(hex(0xf4f1ea), hex(0xe6dfd2), d)
		}
		return mix(hex(0x1d1d22), hex(0x3a3a44), d)
	}},
	{"柠檬", func(x, y float64) color.RGBA {
		c := mix(hex(0xfff3b0), hex(0xffd84d), (x+y)/2)
		d := math.Hypot(x-0.42, y-0.46)
		c = mix(c, hex(0x2e7d32), 1-smooth(0.3, 0.012, d))
		c = mix(c, hex(0xf9e04b), 1-smooth(0.27, 0.012, d))
		seg := math.Mod(math.Atan2(y-0.46, x-0.42)/(2*math.Pi)*8+8, 1)
		if d < 0.25 && (seg < 0.04 || seg > 0.96) {
			c = mix(c, hex(0xfffbe0), 0.85)
		}
		return c
	}},
	{"深海", func(x, y float64) color.RGBA {
		d := math.Hypot(x-0.5, y-0.15)
		c := mix(hex(0x1f7ea8), hex(0x04122a), d*1.2)
		for i := range 9 {
			fi := float64(i)
			bx := 0.12 + math.Mod(fi*0.37, 0.8)
			by := 0.2 + math.Mod(fi*0.53, 0.75)
			r := 0.015 + 0.01*math.Mod(fi*1.3, 2)
			bd := math.Hypot(x-bx, y-by)
			if math.Abs(bd-r) < 0.004 {
				c = mix(c, hex(0xbfe9ff), 0.7)
			}
		}
		return c
	}},
	{"霓虹", func(x, y float64) color.RGBA {
		t := math.Mod((x+y)*6, 1)
		c := hex(0x12061f)
		if t < 0.18 {
			c = mix(hex(0xff3cac), hex(0x2b86c5), (x+y)/2)
		}
		d := math.Hypot(x-0.5, y-0.5)
		return mix(c, hex(0x12061f), smooth(0.55, 0.2, d)*0.6)
	}},
}

// squareJPEG crops a picture to its middle square and scales it to the
// charger's screen.
func squareJPEG(data []byte) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	crop := image.Rect(b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2, 0, 0)
	crop.Max = crop.Min.Add(image.Pt(side, side))
	dst := image.NewRGBA(image.Rect(0, 0, coverSize, coverSize))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, crop, draw.Src, nil)
	return encode(dst), nil
}
