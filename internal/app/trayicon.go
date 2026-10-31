package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Tray icon geometry, in pixels of a 2× menu bar: MyGo scales the image
// to the bar's 18 points.
const (
	iconW, iconH = 50, 36
	// levelSteps quantizes the battery's fill, so the icon is redrawn only
	// when it visibly moves.
	levelSteps = 20
)

// TrayIcon draws the menu bar icon: a battery filled to level (0…1; below
// 0 for no reading, an empty outline), with a bolt cut out while it
// charges. Ink on transparent, for a template image the menu bar tints.
func TrayIcon(level float64, charging bool) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, iconW, iconH))
	// The body, its nub, and the fill inside.
	bx0, by0, bx1, by1 := 3.0, 9.0, 43.0, 27.0
	const stroke, r = 2.6, 5.0
	inset := stroke + 1.8
	fx0, fy0, fy1 := bx0+inset, by0+inset, by1-inset
	fx1 := fx0 + (bx1-inset-fx0)*math.Max(level, 0)
	inBox := func(x, y, x0, y0, x1, y1, rad float64) bool {
		if x < x0 || x > x1 || y < y0 || y > y1 {
			return false
		}
		cx := math.Min(math.Max(x, x0+rad), x1-rad)
		cy := math.Min(math.Max(y, y0+rad), y1-rad)
		return math.Hypot(x-cx, y-cy) <= rad
	}
	bolt := [][2]float64{{25, 10.5}, {17.5, 19.5}, {22.5, 19.5}, {20.5, 26}, {28.5, 16.5}, {23.5, 16.5}}
	for y := 0; y < iconH; y++ {
		for x := 0; x < iconW; x++ {
			hits := 0
			for sy := 0; sy < 4; sy++ {
				for sx := 0; sx < 4; sx++ {
					px := float64(x) + (float64(sx)+0.5)/4
					py := float64(y) + (float64(sy)+0.5)/4
					outer := inBox(px, py, bx0, by0, bx1, by1, r)
					inner := inBox(px, py, bx0+stroke, by0+stroke, bx1-stroke, by1-stroke, r-stroke)
					on := outer && !inner
					on = on || inBox(px, py, bx1+1.6, 14.5, bx1+4.6, 21.5, 1.4)
					if level > 0 && inBox(px, py, fx0, fy0, math.Max(fx1, fx0+2.4), fy1, 2) {
						on = true
					}
					if charging && inPolygon(px, py, bolt) {
						// A clear bolt, ringed by a clear gap over the fill.
						on = false
					}
					if charging && !inPolygon(px, py, bolt) && nearPolygon(px, py, bolt, 1.6) && inner {
						on = false
					}
					if charging && inPolygonShrunk(px, py, bolt) {
						on = true
					}
					if on {
						hits++
					}
				}
			}
			if hits > 0 {
				img.SetNRGBA(x, y, color.NRGBA{A: uint8(math.Round(float64(hits) / 16 * 255))})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img) // encoding an in-memory image cannot fail
	return buf.Bytes()
}

// blankIcon is a sliver of nothing, for a title without an icon: a tray
// item needs an image, and an empty one is refused.
func blankIcon() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 1, iconH))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func inPolygon(x, y float64, pts [][2]float64) bool {
	in := false
	for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
		xi, yi, xj, yj := pts[i][0], pts[i][1], pts[j][0], pts[j][1]
		if (yi > y) != (yj > y) && x < (xj-xi)*(y-yi)/(yj-yi)+xi {
			in = !in
		}
	}
	return in
}

// inPolygonShrunk is the bolt drawn in ink inside its clear cut, a
// little smaller, so it reads on an empty battery too.
func inPolygonShrunk(x, y float64, pts [][2]float64) bool {
	var cx, cy float64
	for _, p := range pts {
		cx += p[0]
		cy += p[1]
	}
	cx /= float64(len(pts))
	cy /= float64(len(pts))
	const k = 1 / 0.78
	return inPolygon(cx+(x-cx)*k, cy+(y-cy)*k, pts)
}

func nearPolygon(x, y float64, pts [][2]float64, d float64) bool {
	for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
		if segDist(x, y, pts[j], pts[i]) <= d {
			return true
		}
	}
	return false
}

func segDist(x, y float64, a, b [2]float64) float64 {
	dx, dy := b[0]-a[0], b[1]-a[1]
	t := ((x-a[0])*dx + (y-a[1])*dy) / (dx*dx + dy*dy)
	t = math.Min(math.Max(t, 0), 1)
	return math.Hypot(x-(a[0]+t*dx), y-(a[1]+t*dy))
}
