package cover

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func writePNG(t *testing.T, dir, name string, w, h int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCropAndSize(t *testing.T) {
	src := t.TempDir()
	for _, sz := range [][2]int{{400, 200}, {100, 300}, {240, 240}} {
		img := image.NewRGBA(image.Rect(0, 0, sz[0], sz[1]))
		sq := CropSquare(img).Bounds()
		if sq.Dx() != sq.Dy() || sq.Dx() != min(sz[0], sz[1]) {
			t.Errorf("%v crop %v", sz, sq)
		}
		data, err := EncodeCover(img)
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width != 240 || cfg.Height != 240 {
			t.Errorf("%v -> %v %v", sz, cfg, err)
		}
	}
	_ = src
}

func TestCropIsCentered(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 300, 100))
	if r := CropSquare(img).Bounds(); r != image.Rect(100, 0, 200, 100) {
		t.Fatal(r)
	}
}

func TestAddCRCAndIDs(t *testing.T) {
	dir := t.TempDir()
	lib, err := Open(filepath.Join(dir, "lib"))
	if err != nil {
		t.Fatal(err)
	}
	a, err := lib.Add(writePNG(t, dir, "sunset.png", 300, 200), "")
	if err != nil {
		t.Fatal(err)
	}
	if a.ID != 1001 || a.Name != "sunset" {
		t.Fatalf("%+v", a)
	}
	data, err := lib.Bytes(a.ID)
	if err != nil || CRC(data) != a.Hash || a.Size != len(data) {
		t.Fatalf("crc/size mismatch: %v", err)
	}
	b, _ := lib.Add(writePNG(t, dir, "b.png", 50, 50), "Second")
	if b.ID != 1002 || b.Name != "Second" {
		t.Fatalf("%+v", b)
	}
	if err := lib.Remove(b.ID); err != nil {
		t.Fatal(err)
	}
	// reopen: id 1002 must not be reused
	lib, err = Open(filepath.Join(dir, "lib"))
	if err != nil {
		t.Fatal(err)
	}
	c, _ := lib.Add(writePNG(t, dir, "c.png", 50, 50), "c")
	if c.ID != 1003 {
		t.Fatalf("id reuse: %d", c.ID)
	}
	if l := lib.List(); len(l) != 2 || l[0].ID != 1003 {
		t.Fatalf("list %+v", l)
	}
	if _, err := lib.Bytes(1002); err != ErrNotFound {
		t.Fatalf("removed bytes: %v", err)
	}
	if _, err := os.Stat(lib.Path(1002)); !os.IsNotExist(err) {
		t.Fatal("file not removed")
	}
}

func TestRenamePersists(t *testing.T) {
	dir := t.TempDir()
	lib, _ := Open(dir)
	it, _ := lib.Add(writePNG(t, t.TempDir(), "x.png", 10, 10), "x")
	if err := lib.Rename(it.ID, "new"); err != nil {
		t.Fatal(err)
	}
	if lib.Rename(9999, "n") != ErrNotFound {
		t.Fatal("want ErrNotFound")
	}
	lib, _ = Open(dir)
	if got, _ := lib.Get(it.ID); got.Name != "new" {
		t.Fatalf("%+v", got)
	}
}

func TestAddJPEGAndThumb(t *testing.T) {
	dir := t.TempDir()
	lib, _ := Open(dir)
	data, _ := EncodeCover(image.NewRGBA(image.Rect(0, 0, 60, 60)))
	it, err := lib.AddJPEG(data, "cloud", 70000, 0xdeadbeef)
	if err != nil || it.Hash != 0xdeadbeef {
		t.Fatalf("%+v %v", it, err)
	}
	if _, err := lib.AddJPEG(data, "dup", 70000, 1); err == nil {
		t.Fatal("duplicate id accepted")
	}
	if _, err := lib.AddJPEG([]byte("nope"), "bad", 5, 1); err == nil {
		t.Fatal("non-jpeg accepted")
	}
	p, err := lib.ThumbnailPath(70000)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.Open(p)
	cfg, err := jpeg.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != thumbSide {
		t.Fatalf("%v %v", cfg, err)
	}
	// local allocation stays below the 16-bit limit and unaffected by large imported ids
	n, _ := lib.Add(writePNG(t, t.TempDir(), "n.png", 10, 10), "n")
	if n.ID != 1001 {
		t.Fatalf("id %d", n.ID)
	}
}

func TestUnsupportedFormat(t *testing.T) {
	p := filepath.Join(t.TempDir(), "junk.heic")
	os.WriteFile(p, []byte("not an image"), 0o644)
	lib, _ := Open(t.TempDir())
	if _, err := lib.Add(p, ""); err == nil {
		t.Fatal("want error")
	}
}

func TestReopenKeepsLocalAllocationWithLargeImportedID(t *testing.T) {
	dir := t.TempDir()
	lib, _ := Open(dir)
	data, _ := EncodeCover(image.NewRGBA(image.Rect(0, 0, 60, 60)))
	if _, err := lib.AddJPEG(data, "cloud", 70000, 1); err != nil {
		t.Fatal(err)
	}
	lib, _ = Open(dir)
	it, err := lib.Add(writePNG(t, t.TempDir(), "n.png", 10, 10), "n")
	if err != nil || it.ID != 1001 {
		t.Fatalf("%+v %v", it, err)
	}
}
