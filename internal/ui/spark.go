package ui

import (
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-desktop/internal/store"
)

// sparkWindow is the span a sparkline shows, newest at the right.
const sparkWindow = store.HistoryLen * time.Second

// sparkline draws the history h, h tall, over the last sparkWindow:
// a line in up, the area under it fading out. Signed histories (a power
// bank's flow) draw what is below zero in down, mirrored about a zero
// line. lo is the smallest top of the scale, so idle noise stays flat.
func sparkline(c *ui.Context, k *Tokens, key string, h []store.Sample, height float32, up, down ui.Color, signed bool, lo float64) ui.Element {
	box := ui.Box(c.Key(key)).Height(height).FillWidth()
	now := c.Now()
	box.Draw(func(p *ui.Painter, r ui.Rect) {
		top := lo
		for _, s := range h {
			top = math.Max(top, math.Abs(s.Value))
		}
		top *= 1.12
		zero := r.Y + r.H - 1
		scale := float64(r.H - 2)
		if signed {
			zero = r.Y + r.H/2
			scale = float64(r.H/2 - 1)
			p.Line(r.X, zero, r.X+r.W, zero, 1, k.Line)
		} else {
			p.Line(r.X, zero+0.5, r.X+r.W, zero+0.5, 1, k.Line)
		}
		if len(h) < 2 {
			return
		}
		pts := make([][2]float32, 0, len(h))
		for _, s := range h {
			age := now.Sub(s.At)
			if age > sparkWindow {
				continue
			}
			x := r.X + r.W*float32(1-float64(age)/float64(sparkWindow))
			y := zero - float32(s.Value/top*scale)
			pts = append(pts, [2]float32{x, y})
		}
		if len(pts) < 2 {
			return
		}
		area := (&ui.Path{}).MoveTo(pts[0][0], zero)
		line := (&ui.Path{}).MoveTo(pts[0][0], pts[0][1])
		for _, pt := range pts {
			area.LineTo(pt[0], pt[1])
		}
		for _, pt := range pts[1:] {
			line.LineTo(pt[0], pt[1])
		}
		area.LineTo(pts[len(pts)-1][0], zero).Close()
		half := func(clip ui.Rect, col ui.Color, angle float32) {
			p.Clip(clip, 0, func() {
				p.FillPathGradient(area, ui.LinearGradient{From: col.Alpha(0.28), To: col.Alpha(0), Angle: angle})
				p.StrokePath(line, 1.6, col)
			})
		}
		half(ui.Rect{X: r.X, Y: r.Y - 2, W: r.W, H: zero - r.Y + 2}, up, 180)
		if signed {
			half(ui.Rect{X: r.X, Y: zero, W: r.W, H: r.Y + r.H - zero + 2}, down, 0)
		}
	})
	return box
}
