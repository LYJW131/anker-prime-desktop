package ui

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/LYJW131/anker-prime-mac/internal/cover"
	"github.com/LYJW131/anker-prime-mac/internal/link"
	"github.com/LYJW131/anker-prime-mac/internal/store"
	"github.com/LYJW131/anker-prime-mac/locales"
)

// coversForm is the cover page's state between frames.
type coversForm struct {
	renaming   *cover.Item
	renameOpen bool
	name       string
	removing   *cover.Item
	removeOpen bool
	// jobSeen is the picture of the job the store last held, since
	// jobFrom.
	jobSeen int
	jobFrom time.Time
}

// jobShown is the cover job the views show: a selection the charger
// answers at once shows only its answer, a push its progress once it ran
// statusDelay, a failure at once.
func (a *App) jobShown(c *ui.Context) *link.CoverJob {
	f := &a.covers
	job := a.Store.CoverJob
	if job == nil {
		f.jobSeen = 0
		return nil
	}
	if job.ID != f.jobSeen {
		f.jobSeen, f.jobFrom = job.ID, c.Now()
	}
	if job.Err != nil || waited(c, true, f.jobFrom) {
		return job
	}
	return nil
}

// thumbKey names a decoded picture: the same id with other pixels is
// another picture.
type thumbKey struct {
	id   int
	hash uint32
}

// thumb is a library picture, decoded once.
func (a *App) thumb(it cover.Item) *ui.Bitmap {
	key := thumbKey{it.ID, it.Hash}
	if a.thumbs == nil {
		a.thumbs = map[thumbKey]*ui.Bitmap{}
	}
	if b, ok := a.thumbs[key]; ok {
		return b
	}
	var b *ui.Bitmap
	if data := a.Dev.CoverData(it); len(data) > 0 {
		b, _ = ui.DecodeBitmap(data)
	}
	a.thumbs[key] = b
	return b
}

// currentCover is the library's picture a charger shows. The charger
// reports only the low 16 bits of its id.
func (a *App) currentCover(d dev) (cover.Item, bool) {
	id := d.charger().ScreensaverID
	if id == nil {
		return cover.Item{}, false
	}
	for _, it := range a.Store.Covers {
		if it.ID&0xffff == *id&0xffff {
			return it, true
		}
	}
	return cover.Item{}, false
}

// coverImage is a picture of the library at size, rounded by radius, or
// its placeholder.
func (a *App) coverImage(c *ui.Context, k *Tokens, it cover.Item, size, radius float32) ui.Element {
	if b := a.thumb(it); b != nil {
		img := ui.Image(c, b).Fit(ui.Cover).Radius(radius)
		if size > 0 {
			img.Size(size, size)
		}
		return img
	}
	box := ui.Box(c).Radius(radius).Background(k.Track).Center().Children(func() {
		ui.Icon(c, icCover).FontSize(18).TextColor(k.Faint)
	})
	if size > 0 {
		box.Size(size, size)
	}
	return box
}

// coverChip is a charger's current cover as a small picture and its name.
func (a *App) coverChip(c *ui.Context, k *Tokens, d dev, size float32) {
	ui.Row(c.Key("cover-chip")).Gap(10).AlignItems(ui.Center).MinWidth(0).Shrink(1).Children(func() {
		it, ok := a.currentCover(d)
		if !ok {
			ui.Box(c).Size(size, size).Radius(radiusInner).Background(k.Track).Center().Children(func() {
				ui.Icon(c, icCover).FontSize(15).TextColor(k.Faint)
			})
			ui.Text(c, a.currentCoverName(d)).FontSize(13).FontWeight(600).TextColor(k.Faint).SingleLine()
			return
		}
		a.coverImage(c, k, it, size, radiusInner)
		ui.Text(c, it.Name).FontSize(13).FontWeight(600).SingleLine().Shrink(1).Ellipsis("…")
	})
}

// coverTile is the smallest a library tile gets before the grid drops a
// column.
const coverTile = 200

// coversTab is a charger's covers: the library, a click on a picture
// shows it on this charger.
func (a *App) coversTab(c *ui.Context, k *Tokens, d dev, s devState) {
	st := a.Store
	if len(st.Covers) == 0 {
		a.coversEmpty(c, k)
		return
	}
	w := a.winW
	if a.SidebarShown() {
		w -= sidebarWidth
	}
	inner := w - 2*pagePad
	cols := max(1, int(math.Floor(float64(inner+14)/float64(coverTile+14))))
	ui.Grid(c.Key("library")).Columns(cols).Gap(14).Children(func() {
		for _, it := range st.Covers {
			a.coverTileView(c, k, d, it, s.live)
		}
	})
}

// addCoverButton adds a picture to the library.
func (a *App) addCoverButton(c *ui.Context, k *Tokens) ui.Element {
	b := button(c, k, buttonPrimary, locales.CoversAdd(), icPlus)
	if b.Clicked() {
		a.Act.ChooseImage(func(path string) { a.Dev.AddCover(path) })
	}
	return b
}

// coversEmpty is the library before its first picture.
func (a *App) coversEmpty(c *ui.Context, k *Tokens) {
	ui.Column(c.Key("empty")).Padding(56, padCard).Gap(10).Radius(radiusCard).Background(k.Raise).AlignItems(ui.Center).Children(func() {
		tile(c, icCover, k.TileCover, 44).Margin(0, 0, 6, 0)
		ui.Text(c, locales.CoversEmpty()).FontSize(17).FontWeight(700)
		a.addCoverButton(c, k).Margin(6, 0, 0, 0)
	})
}

// coverTileView is one picture of the library: a click shows it on the
// charger; its context menu renames or removes it.
func (a *App) coverTileView(c *ui.Context, k *Tokens, d dev, it cover.Item, connected bool) {
	s := a.Store
	cur, ok := a.currentCover(d)
	current := ok && cur.ID == it.ID
	job := a.jobShown(c)
	pushing := job != nil && job.ID == it.ID && job.Err == nil
	failed := job != nil && job.ID == it.ID && job.Err != nil
	busy := s.CoverJob != nil && s.CoverJob.Err == nil
	t := ui.ButtonBase(c.Key(it.ID)).Padding(padControl * 2).Gap(10).Radius(radiusCard).Column().
		AlignItems(ui.Stretch).Justify(ui.Start).MinWidth(0).Label(it.Name)
	switch {
	case t.Pressed():
		t.Background(k.Track)
	case t.Hovered():
		t.Background(k.Wash)
	}
	if t.Clicked() && !current && !busy {
		if connected {
			a.Dev.SelectCover(d.ID(), it)
		} else {
			s.Note(store.NoticeCoverFailed, it.Name, link.ErrNotConnected)
		}
	}
	t.ContextMenu(func(m *ui.Menu) {
		if m.Item(locales.CoverSet()).Disabled(current || busy || !connected).Chosen() {
			a.Dev.SelectCover(d.ID(), it)
		}
		if m.Item(locales.CoverRename()).Chosen() {
			cp := it
			a.covers.renaming, a.covers.renameOpen, a.covers.name = &cp, true, it.Name
		}
		m.Separator()
		if m.Item(locales.CoverRemove()).Disabled(pushing).Chosen() {
			cp := it
			a.covers.removing, a.covers.removeOpen = &cp, true
		}
	})
	t.Cursor(ui.CursorPointer).Children(func() {
		// The picture is a control inside the tile: its radius is the
		// tile's less the room between them.
		r := float32(radiusCard - padControl*2)
		ui.Box(c.Key("pic")).AspectRatio(1).FillWidth().Children(func() {
			a.coverImage(c, k, it, 0, r).Absolute().Top(0).Left(0).Right(0).Bottom(0)
			if current {
				// A ring around the picture, concentric with it.
				ui.Box(c).Absolute().Top(-padControl).Left(-padControl).Right(-padControl).Bottom(-padControl).
					Radius(r+padControl).Border(2, k.Accent).PassThrough()
				pill(c, k, pillOK, icCheck, locales.CoverCurrent()).Background(k.Raise).Absolute().Top(10).Left(10)
			}
			if pushing || failed {
				a.pushOverlay(c, k, r, failed)
			}
		})
		ui.Column(c).Gap(3).PaddingX(4).Children(func() {
			ui.Text(c, it.Name).FontSize(13).FontWeight(600).SingleLine().Ellipsis("…")
			mono(c, fmt.Sprintf("#%d · %s", it.ID, sizeText(it.Size)), 11).TextColor(k.Faint)
		})
	})
}

// pushOverlay covers a picture on its way to the charger: a scrim, the
// chunks sent of all, and a bar along its foot.
func (a *App) pushOverlay(c *ui.Context, k *Tokens, r float32, failed bool) {
	job := a.jobShown(c)
	ui.Column(c.Key("push")).Absolute().Top(0).Left(0).Right(0).Bottom(0).Radius(r).Background(ui.RGBA(8, 9, 14, 0.55)).
		Padding(14).Gap(8).Justify(ui.End).Children(func() {
		white := ui.RGB(255, 255, 255)
		switch {
		case failed:
			ui.Text(c, locales.CoverSendFailed()).FontSize(13).FontWeight(650).TextColor(white)
			ui.Text(c, ErrText(job.Err)).FontSize(11).TextColor(white.Alpha(0.8)).MaxLines(2).Ellipsis("…")
		case job.Total == 0:
			ui.Text(c, locales.CoverSwitching()).FontSize(13).FontWeight(650).TextColor(white)
		default:
			ui.Row(c).AlignItems(ui.Baseline).Children(func() {
				ui.Text(c, locales.CoverSending()).FontSize(12.5).FontWeight(650).TextColor(white)
				ui.Spacer(c)
				widest := fmt.Sprintf("%d/%d", job.Total, job.Total)
				slotText(c, func(s string) ui.Element { return mono(c, s, 11).TextColor(white.Alpha(0.85)) },
					fmt.Sprintf("%d/%d", job.Done, job.Total), widest, ui.End)
			})
			frac := float64(job.Done) / float64(max(job.Total, 1))
			bar := ui.Row(c.Key("bar")).Height(4).Radius(radiusFull).Background(white.Alpha(0.22)).AlignItems(ui.Stretch).Clip()
			v := bar.AnimateTo("v", float32(frac), ui.Snappy)
			bar.Children(func() {
				if v > 0.001 {
					ui.Box(c).Grow(v).Background(white)
				}
				if v < 0.999 {
					ui.Box(c).Grow(1 - v)
				}
			})
		}
	})
}

func sizeText(n int) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	return fmt.Sprintf("%.0f KB", float64(n)/1024)
}

// coverDialogs are the rename and remove dialogs of the library.
func (a *App) coverDialogs(c *ui.Context, k *Tokens) {
	f := &a.covers
	if f.renaming != nil {
		it := *f.renaming
		save := func() {
			if f.name != "" && f.name != it.Name {
				a.Dev.RenameCover(it.ID, f.name)
			}
			f.renameOpen = false
		}
		dialog(c, k, "rename", &f.renameOpen, locales.RenameTitle(), "", func() {
			ui.Row(c).Children(func() {
				_, in := fieldBox(c, k, "rename-field", &f.name, locales.RenamePlaceholder(), false)
				in.AutoFocus()
				if in.Submitted() {
					save()
				}
			})
		}, func() {
			if button(c, k, buttonPlain, locales.Cancel(), nil).Clicked() {
				f.renameOpen = false
			}
			if buttonOff(c, k, buttonPrimary, locales.Save(), nil, f.name == "").Clicked() {
				save()
			}
		})
		if !f.renameOpen {
			f.renaming = nil
		}
	}
	if f.removing != nil {
		it := *f.removing
		dialog(c, k, "remove", &f.removeOpen, locales.ConfirmRemove(it.Name), locales.RemoveCoverBody(), nil, func() {
			if button(c, k, buttonPlain, locales.Cancel(), nil).Clicked() {
				f.removeOpen = false
			}
			if button(c, k, buttonDanger, locales.Remove(), nil).Clicked() {
				a.Dev.RemoveCover(it.ID)
				f.removeOpen = false
			}
		})
		if !f.removeOpen {
			f.removing = nil
		}
	}
}
