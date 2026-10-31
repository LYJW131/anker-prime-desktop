package ui

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
)

// label is the design's small caps label: 11 px, spaced out.
func label(c *ui.Context, k *Tokens, s string) ui.Element {
	return ui.Text(c, strings.ToUpper(s)).FontSize(11).FontWeight(600).LetterSpacing(0.77).TextColor(k.Faint).SingleLine()
}

// mono sets a number, a serial or another fact in the monospace face with
// digits of one width.
func mono(c *ui.Context, s string, size float32) ui.Element {
	return ui.Text(c, s).Font(MonoFont).FontSize(size).FontWeight(500).FontFeatures("tnum").SingleLine()
}

// display is a big title or number in the system font's heaviest weight,
// tracked tight, its digits of one width.
func display(c *ui.Context, s string, size float32) ui.Element {
	return ui.Text(c, s).FontSize(size).FontWeight(800).LetterSpacing(-0.02 * size).LineHeight(1.05).FontFeatures("tnum")
}

// bigValue is a big number with its small unit after it, on one baseline,
// in a box as wide as widest would be: the number changes every second,
// and nothing beside it moves.
func bigValue(c *ui.Context, k *Tokens, value, unit, widest string, size float32, col ui.Color) ui.Element {
	return bigValueTail(c, k, value, unit, widest, size, col, "")
}

// bigValueTail is bigValue with a faint tail on its baseline, as the
// scale it is out of.
func bigValueTail(c *ui.Context, k *Tokens, value, unit, widest string, size float32, col ui.Color, tail string) ui.Element {
	row := func(v string, invisible bool) ui.Element {
		r := ui.Row(c).AlignItems(ui.Baseline).Gap(size * 0.08)
		r.Children(func() {
			parts := []ui.Element{display(c, v, size).SingleLine()}
			if col.A != 0 {
				parts[0].TextColor(col)
			}
			// The empty value goes without its unit.
			if unit != "" && (invisible || v != none) {
				parts = append(parts, ui.Text(c, unit).FontSize(size*0.44).FontWeight(600).TextColor(k.Muted).SingleLine())
			}
			if tail != "" {
				parts = append(parts, mono(c, tail, max(11, size*0.24)).TextColor(k.Faint).Margin(0, 0, 0, size*0.12))
			}
			if invisible {
				for _, p := range parts {
					p.Invisible()
				}
			}
		})
		return r
	}
	return ui.Box(c).Children(func() {
		if widest == "" {
			widest = value
		}
		row(widest, true)
		row(value, false).Absolute().Top(0).Left(0)
	})
}

// slotText is text in a box as wide as widest, aligned by align, so a
// value that changes width moves nothing beside it.
func slotText(c *ui.Context, el func(s string) ui.Element, value, widest string, align ui.Align) ui.Element {
	return ui.Box(c).Children(func() {
		el(widest).Invisible()
		el(value).TextAlign(align).Absolute().Top(0).Left(0).Right(0)
	})
}

// pillKind colors a status pill.
type pillKind int

const (
	pillRun pillKind = iota
	pillOK
	pillBad
	pillWait
	pillWarn
)

func (p pillKind) colors(k *Tokens) (bg, fg ui.Color) {
	switch p {
	case pillOK:
		return k.OKSoft, k.OK
	case pillBad:
		return k.BadSoft, k.Bad
	case pillWait:
		return k.Track, k.Muted
	case pillWarn:
		return k.WarnSoft, k.Warn
	}
	return k.AccentSoft, k.Accent
}

// pill is a status: icon, word and color together, never color alone.
func pill(c *ui.Context, k *Tokens, kind pillKind, ic *ui.SVG, text string) ui.Element {
	bg, fg := kind.colors(k)
	return ui.Row(c).Height(20).PaddingX(8).Gap(5).Radius(radiusFull).Background(bg).AlignItems(ui.Center).Children(func() {
		if ic != nil {
			ui.Icon(c, ic).FontSize(11).TextColor(fg)
		}
		ui.Text(c, text).FontSize(11).FontWeight(600).TextColor(fg).SingleLine()
	})
}

// dot is a status dot.
func dot(c *ui.Context, col ui.Color, size float32) ui.Element {
	return ui.Box(c).Size(size, size).Radius(radiusFull).Background(col)
}

// tile is the colored icon square of the sidebar and the cards' headings.
func tile(c *ui.Context, ic *ui.SVG, col ui.Color, size float32) ui.Element {
	return ui.Box(c).Size(size, size).Radius(radiusInner).Background(col).Center().Children(func() {
		ui.Icon(c, ic).FontSize(size * 0.6).TextColor(ui.RGB(255, 255, 255))
	})
}

// buttonKind styles a button.
type buttonKind int

const (
	buttonPlain buttonKind = iota
	buttonPrimary
	buttonGhost
	// buttonPill is Mole's big white pill, the one action of a bar.
	buttonPill
	buttonDanger
	// buttonDestructive is a plain button that does something that cannot
	// be undone: its word in the danger color.
	buttonDestructive
)

// button is the design's button; it returns the base, whose Clicked reports
// a press.
func button(c *ui.Context, k *Tokens, kind buttonKind, text string, ic *ui.SVG) ui.Element {
	return buttonOff(c, k, kind, text, ic, false)
}

// buttonOff is button, disabled when off. A button disabled after it is
// built still reports the hover and press it styles itself by, so a button
// that can be disabled is built with this.
func buttonOff(c *ui.Context, k *Tokens, kind buttonKind, text string, ic *ui.SVG, off bool) ui.Element {
	b := ui.ButtonBase(c).Disabled(off).Height(30).PaddingX(12).Gap(6).Radius(radiusControl).AlignItems(ui.Center).Justify(ui.Center)
	fg := k.Fg
	switch kind {
	case buttonPrimary:
		b.Background(k.Accent)
		fg = k.AccentInk
	case buttonDanger:
		b.Background(k.Bad)
		fg = ui.RGB(255, 255, 255)
	case buttonGhost:
		// No fill at rest, so the fill is the only sign of a hover or a
		// press.
		fg = k.Muted
		switch {
		case b.Pressed():
			b.Background(k.Track.Mix(k.Fg, 0.08))
			fg = k.Fg
		case b.Hovered():
			b.Background(k.Track)
			fg = k.Fg
		}
	case buttonDestructive:
		b.Background(k.Raise2).Border(1, k.Line2)
		fg = k.Bad
	case buttonPill:
		b.Height(36).PaddingX(18).Radius(radiusFull).Background(k.Pill)
		fg = k.PillInk
	default:
		b.Background(k.Raise2).Border(1, k.Line2)
	}
	if off {
		b.Opacity(0.45)
	} else if b.Hovered() && kind != buttonGhost {
		b.Opacity(0.92)
	}
	if text == "" {
		b.PaddingX(0).Width(30)
	}
	b.Cursor(ui.CursorPointer).Children(func() {
		if ic != nil {
			ui.Icon(c, ic).FontSize(14).TextColor(fg)
		}
		if text != "" {
			w := 500
			if kind == buttonPrimary || kind == buttonPill || kind == buttonDanger {
				w = 650
			}
			ui.Text(c, text).FontSize(12.5).FontWeight(w).TextColor(fg).SingleLine()
		}
	})
	return b
}

// iconButton is a small ghost button of one icon, labeled for assistive
// technology and the tooltip.
func iconButton(c *ui.Context, k *Tokens, ic *ui.SVG, name string, off bool) ui.Element {
	b := ui.ButtonBase(c).Disabled(off).Size(26, 26).Radius(radiusInner).Center().Label(name).Tooltip(name)
	fg := k.Muted
	switch {
	case off:
		fg = k.Muted.Alpha(0.35)
	case b.Pressed():
		b.Background(k.Track.Mix(k.Fg, 0.08))
		fg = k.Fg
	case b.Hovered():
		b.Background(k.Track)
		fg = k.Fg
	}
	b.Cursor(ui.CursorPointer).Children(func() {
		ui.Icon(c, ic).FontSize(14).TextColor(fg)
	})
	return b
}

// segmented is the design's segmented control: the chosen segment white on
// dark (ink on light), the others muted.
func segmented(c *ui.Context, k *Tokens, key string, selected *int, labels []string) ui.Element {
	return segmentedLang(c, k, key, selected, labels, nil)
}

// segmentedLang is segmented with each label in a language of its own
// (langs[i], "" for the app's), as a menu of languages names each in
// itself: Han text takes the glyphs, and the widths, of its language.
func segmentedLang(c *ui.Context, k *Tokens, key string, selected *int, labels, langs []string) ui.Element {
	s := ui.SegmentedBase(c.Key(key), selected, len(labels))
	s.Track.Padding(padControl).Radius(radiusControl).Background(k.Raise2).Border(1, k.Line).Children(func() {
		for i, l := range labels {
			seg := s.Segment(i).PaddingX(10).PaddingY(3).Radius(radiusInner).Label(l)
			fg, weight := k.Muted, 500
			if i == *selected {
				seg.Background(k.Pill).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.25))
				fg, weight = k.PillInk, 600
			} else if seg.Hovered() {
				fg = k.Fg
			}
			// Each segment keeps the width of its bold label, so choosing one
			// moves neither the others nor the track's edge.
			seg.Cursor(ui.CursorPointer).Children(func() {
				box := ui.Box(c).Children(func() {
					ui.Text(c, l).FontSize(12).FontWeight(600).SingleLine().Invisible()
					ui.Text(c, l).FontSize(12).FontWeight(weight).TextColor(fg).SingleLine().TextAlign(ui.Center).
						Absolute().Top(0).Left(0).Right(0)
				})
				if i < len(langs) && langs[i] != "" {
					box.Lang(langs[i])
				}
			})
		}
	})
	return s.Track
}

// toggle is a switch in the design's look.
func toggle(c *ui.Context, k *Tokens, key string, on *bool, name string) ui.Element {
	sw := ui.SwitchBase(c.Key(key), on).Width(34).Height(20).Radius(radiusFull).Label(name)
	// The track's color moves with the knob, on the same curve.
	track := k.Track
	if *on {
		track = k.Accent
	}
	sw.Background(sw.AnimateColor("track", track, ui.Snappy))
	x := sw.AnimateTo("knob", map[bool]float32{true: 16, false: 2}[*on], ui.Snappy)
	sw.Cursor(ui.CursorPointer).Children(func() {
		ui.Box(c).Size(16, 16).Radius(radiusFull).Background(ui.RGB(255, 255, 255)).
			Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.2)).Absolute().Top(2).Left(x)
	})
	return sw
}

// focusRing draws the platform's focus ring, MyGo's own, around el rounded
// by radius: for a box whose focused element is a part of it, as a field's
// input. A text field rings whenever it has the focus, as macOS's do.
func focusRing(el ui.Element, radius float32) {
	el.DrawOver(func(p *ui.Painter, r ui.Rect) { p.FocusRing(r, [4]float32{radius, radius, radius, radius}) })
}

// card is the Mole card: large radius, no border.
func card(c *ui.Context, k *Tokens) ui.Element {
	return ui.Column(c).Padding(padCard).Gap(12).Radius(radiusCard).Background(k.Raise)
}

// meter is a bar filled to frac (0…1) in col, gliding to a new value.
func meter(c *ui.Context, k *Tokens, key string, frac float64, h float32, col ui.Color) ui.Element {
	frac = min(max(frac, 0), 1)
	bar := ui.Row(c.Key(key)).Height(h).Radius(radiusFull).Background(k.Track).AlignItems(ui.Stretch).Clip()
	v := bar.AnimateTo("v", float32(frac), ui.Snappy)
	bar.Children(func() {
		if v > 0.001 {
			ui.Box(c).Grow(v).Radius(radiusFull).Background(col)
		}
		if v < 0.999 {
			ui.Box(c).Grow(1 - v)
		}
	})
	return bar
}

// fieldBox is a one-line text input in the design's look: the box and the
// input inside it.
func fieldBox(c *ui.Context, k *Tokens, key string, value *string, placeholder string, monoFace bool) (box, in ui.Element) {
	box = ui.Row(c).Grow(1).Height(32).PaddingX(10).Radius(radiusControl).AlignItems(ui.Center).Background(k.Raise2).Border(1, k.Line2)
	box.Children(func() {
		in = ui.TextInputBase(c.Key(key), value).Grow(1).Placeholder(placeholder).Label(placeholder).FocusRing(false)
		if monoFace {
			in.Font(MonoFont).FontSize(12)
		} else {
			in.FontSize(13)
		}
	})
	if in.Focused() {
		focusRing(box, radiusControl)
	}
	return box, in
}

// statusDelay is how long a request runs before the screen says it is under
// way: one that answers sooner shows its answer alone, with no flash of the
// waiting state.
const statusDelay = 250 * time.Millisecond

// waited reports whether the request that is on its way (active) left at
// since at least statusDelay ago: only then does the screen show its waiting
// state, grey a control or dim the last results. Until then it asks for the
// frame that shows them, so a quick answer shows its result alone.
func waited(c *ui.Context, active bool, since time.Time) bool {
	if !active {
		return false
	}
	d := c.Now().Sub(since)
	if d >= statusDelay {
		return true
	}
	c.After(statusDelay - d)
	return false
}

// dialog is the design's modal: a title, one line of consequence, and the
// buttons, Cancel first.
func dialog(c *ui.Context, k *Tokens, key string, open *bool, title, body string, extra func(), buttons func()) {
	ui.DialogBase(c.Key(key), open, func(back, panel ui.Element) {
		back.Background(ui.RGBA(0, 0, 0, 0.45))
		panel.Width(420).Padding(padCard+4).Gap(12).Radius(radiusCard).Background(k.Raise).Border(1, k.Line2).
			Shadow(0, 18, 48, 0, ui.RGBA(0, 0, 0, 0.4))
		ui.Text(c, title).FontSize(15).FontWeight(650)
		if body != "" {
			ui.Text(c, body).FontSize(12.5).TextColor(k.Muted).LineHeight(1.5)
		}
		if extra != nil {
			extra()
		}
		ui.Row(c).Gap(8).Justify(ui.End).Padding(6, 0, 0, 0).Children(buttons)
	})
}
