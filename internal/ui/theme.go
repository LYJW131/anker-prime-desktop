package ui

import (
	"github.com/egoist/mygo/ui"
)

// Tokens are the design's colors: a near-black ground, one periwinkle
// accent, status colors only for status. Light mode is designed
// separately, not inverted.
type Tokens struct {
	Dark bool
	// Ink is behind everything; Ground the content area; Raise and Raise2
	// cards and fields above it.
	Ink, Ground, Raise, Raise2 ui.Color
	// Line and Line2 are hairlines, the second stronger.
	Line, Line2                   ui.Color
	Fg, Muted, Faint              ui.Color
	Accent, AccentInk, AccentSoft ui.Color
	OK, Warn, Bad                 ui.Color
	OKSoft, WarnSoft, BadSoft     ui.Color
	// Wash is a faint neutral fill (a card or a row under the pointer)
	// that reads over the window's material as over a solid ground;
	// Track an empty bar, and the stronger fill of a selected row.
	Wash, Track ui.Color
	// Pill is the selected segment and the big action button: white on
	// dark, ink on light.
	Pill, PillInk ui.Color
	// Tile colors are the sidebar's icon squares.
	TileOverview, TileCharger, TilePowerBank, TileCover, TileSettings ui.Color
	// Ports color each port in a share bar and its legend, in the
	// devices' port order (the power bank's dock last).
	Ports [4]ui.Color
}

func rgba(hex string, a float32) ui.Color { return ui.Hex(hex).Alpha(a) }

// DarkTokens are the dark theme, the one the app is designed in first.
var DarkTokens = Tokens{
	Dark: true,
	Ink:  ui.Hex("#0c0d12"), Ground: ui.Hex("#111219"), Raise: ui.Hex("#181a23"), Raise2: ui.Hex("#1f2230"),
	Line: rgba("#eceef8", .09), Line2: rgba("#eceef8", .16),
	Fg: ui.Hex("#eef0f7"), Muted: ui.Hex("#a3a8bb"), Faint: ui.Hex("#878ca1"),
	Accent: ui.Hex("#93a8ff"), AccentInk: ui.Hex("#0c1030"), AccentSoft: rgba("#93a8ff", .16),
	OK: ui.Hex("#62d69a"), Warn: ui.Hex("#f3b75e"), Bad: ui.Hex("#ff8a7c"),
	OKSoft: rgba("#62d69a", .14), WarnSoft: rgba("#f3b75e", .14), BadSoft: rgba("#ff8a7c", .14),
	Wash: rgba("#eceef8", .06), Track: rgba("#eceef8", .10),
	Pill: ui.Hex("#f4f5fa"), PillInk: ui.Hex("#12131a"),
	TileOverview: ui.Hex("#7a5af8"), TileCharger: ui.Hex("#4f7cff"), TilePowerBank: ui.Hex("#2fb36f"),
	TileCover: ui.Hex("#e0507a"), TileSettings: ui.Hex("#8a8f9e"),
	Ports: [4]ui.Color{ui.Hex("#93a8ff"), ui.Hex("#5fd4e8"), ui.Hex("#f59ad6"), ui.Hex("#f3c76b")},
}

// LightTokens are the light theme.
var LightTokens = Tokens{
	Ink: ui.Hex("#eef0f5"), Ground: ui.Hex("#f6f7fa"), Raise: ui.Hex("#ffffff"), Raise2: ui.Hex("#f0f2f7"),
	Line: rgba("#141622", .09), Line2: rgba("#141622", .16),
	Fg: ui.Hex("#161822"), Muted: ui.Hex("#555b6e"), Faint: ui.Hex("#5f6476"),
	Accent: ui.Hex("#3d58e0"), AccentInk: ui.Hex("#ffffff"), AccentSoft: rgba("#3d58e0", .12),
	OK: ui.Hex("#17834b"), Warn: ui.Hex("#9a5c00"), Bad: ui.Hex("#c23a2c"),
	OKSoft: rgba("#17834b", .10), WarnSoft: rgba("#9a5c00", .10), BadSoft: rgba("#c23a2c", .10),
	Wash: rgba("#141622", .05), Track: rgba("#141622", .10),
	Pill: ui.Hex("#161822"), PillInk: ui.Hex("#ffffff"),
	TileOverview: ui.Hex("#7a5af8"), TileCharger: ui.Hex("#4f7cff"), TilePowerBank: ui.Hex("#2fb36f"),
	TileCover: ui.Hex("#e0507a"), TileSettings: ui.Hex("#8a8f9e"),
	Ports: [4]ui.Color{ui.Hex("#3d58e0"), ui.Hex("#0e8aa3"), ui.Hex("#b8418f"), ui.Hex("#a86c00")},
}

// MonoFont is the face of numbers, serials and other facts; the interface
// and the big titles use the system font.
const MonoFont = "JetBrains Mono"

// Corner radii, from the outside in. A box inside another takes the outer
// radius less the room between them (r = R − p), so both arcs share a
// center; the elements of a level share its radius, and none goes under
// radiusInner. Pills and circles stay fully round.
const (
	// radiusCard rounds the window's own boxes: cards, dialogs, the tray
	// panel.
	radiusCard = 24
	// padCard is the room inside a card around what sits in its corners.
	padCard = 16
	// radiusControl rounds what sits in a card or on a screen: buttons,
	// fields, list rows, small panels, port cells, covers.
	radiusControl = radiusCard - padCard
	// padControl is the room inside a control around a box in it.
	padControl = 4
	// radiusInner rounds what sits in a control: segments, chips, tags,
	// icon tiles.
	radiusInner = radiusControl - padControl
	// radiusMenu rounds a menu's or a dialog's list, whose rows are
	// controls.
	radiusMenu = radiusControl + padControl
	radiusFull = 999
)

// applyTheme sets the widgets' theme from the tokens, so the toolkit's own
// controls (inputs, switches, menus) match the app.
func applyTheme(c *ui.Context, k *Tokens) {
	t := *c.Theme()
	t.Dark = k.Dark
	t.Background, t.Surface = k.Ground, k.Raise
	t.SurfaceHover, t.SurfacePressed = k.Raise2, k.Raise2.Mix(k.Fg, 0.06)
	t.Border = k.Line2
	t.Text, t.TextMuted = k.Fg, k.Muted
	t.Accent, t.AccentText = k.Accent, k.AccentInk
	t.AccentHover, t.AccentPressed = k.Accent.Mix(k.Fg, 0.12), k.Accent.Mix(k.Ink, 0.15)
	t.Danger, t.Warning, t.Success = k.Bad, k.Warn, k.OK
	t.Selection, t.Focus = k.AccentSoft, k.Accent
	t.Radius, t.FontSize = radiusControl, 13
	c.SetTheme(&t)
}
