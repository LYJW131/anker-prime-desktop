package cover

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	xdraw "golang.org/x/image/draw"
)

const (
	// FirstID is the first id handed out; ids only grow and are never reused.
	FirstID = 1001
	// MaxID bounds local ids: the charger reports only the low 16 bits.
	MaxID = 65535

	thumbSide = 96
	indexFile = "index.json"
)

// ErrNotFound is returned for an id that is not in the library.
var ErrNotFound = errors.New("cover: not found")

type indexDoc struct {
	NextID int    `json:"next_id"`
	Items  []Item `json:"items"`
}

// Library is a directory of cover JPEGs plus index.json. It is safe for
// concurrent use inside one process only.
type Library struct {
	dir string
	mu  sync.Mutex
	doc indexDoc
}

// Open opens (creating if needed) the library in dir.
func Open(dir string) (*Library, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	l := &Library{dir: dir}
	data, err := os.ReadFile(filepath.Join(dir, indexFile))
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, &l.doc); err != nil {
			return nil, fmt.Errorf("cover: bad %s: %w", indexFile, err)
		}
	}
	if l.doc.NextID < FirstID {
		l.doc.NextID = FirstID
	}
	for _, it := range l.doc.Items {
		// Imported ids above MaxID (cloud ids) must not exhaust local allocation.
		if it.ID >= l.doc.NextID && it.ID <= MaxID {
			l.doc.NextID = it.ID + 1
		}
	}
	return l, nil
}

// Dir returns the library directory.
func (l *Library) Dir() string { return l.dir }

// List returns all items, newest first.
func (l *Library) List() []Item {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := append([]Item(nil), l.doc.Items...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].AddedAt.Equal(out[j].AddedAt) {
			return out[i].AddedAt.After(out[j].AddedAt)
		}
		return out[i].ID > out[j].ID
	})
	return out
}

// Get returns one item.
func (l *Library) Get(id int) (Item, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if i := l.find(id); i >= 0 {
		return l.doc.Items[i], true
	}
	return Item{}, false
}

// Add imports an image file as a new cover. An empty name defaults to the
// file's base name without extension.
func (l *Library) Add(srcPath, name string) (Item, error) {
	img, err := DecodeFile(srcPath)
	if err != nil {
		return Item{}, err
	}
	data, err := EncodeCover(img)
	if err != nil {
		return Item{}, err
	}
	if strings.TrimSpace(name) == "" {
		base := filepath.Base(srcPath)
		name = strings.TrimSuffix(base, filepath.Ext(base))
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	id, err := l.allocID()
	if err != nil {
		return Item{}, err
	}
	return l.insert(id, name, data, CRC(data))
}

// AddJPEG stores an existing JPEG under the given id and hash, for example a
// cloud cover. The bytes are kept as they are.
func (l *Library) AddJPEG(data []byte, name string, id int, hash uint32) (Item, error) {
	if id <= 0 {
		return Item{}, fmt.Errorf("cover: invalid id %d", id)
	}
	if _, err := jpeg.DecodeConfig(bytes.NewReader(data)); err != nil {
		return Item{}, fmt.Errorf("cover: not a JPEG: %w", err)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.find(id) >= 0 {
		return Item{}, fmt.Errorf("cover: id %d already exists", id)
	}
	return l.insert(id, name, data, hash)
}

// Remove deletes a cover. Its id is not reused.
func (l *Library) Remove(id int) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	i := l.find(id)
	if i < 0 {
		return ErrNotFound
	}
	it := l.doc.Items[i]
	l.doc.Items = append(l.doc.Items[:i:i], l.doc.Items[i+1:]...)
	if err := l.save(); err != nil {
		return err
	}
	_ = os.Remove(filepath.Join(l.dir, it.File))
	_ = os.Remove(l.thumbPath(id))
	return nil
}

// Rename changes an item's display name.
func (l *Library) Rename(id int, name string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	i := l.find(id)
	if i < 0 {
		return ErrNotFound
	}
	l.doc.Items[i].Name = name
	return l.save()
}

// Bytes returns the stored JPEG.
func (l *Library) Bytes(id int) ([]byte, error) {
	it, ok := l.Get(id)
	if !ok {
		return nil, ErrNotFound
	}
	return os.ReadFile(filepath.Join(l.dir, it.File))
}

// Path returns the full-size JPEG path for id (it may not exist).
func (l *Library) Path(id int) string {
	return filepath.Join(l.dir, fmt.Sprintf("%d.jpg", id))
}

func (l *Library) thumbPath(id int) string {
	return filepath.Join(l.dir, "thumbs", fmt.Sprintf("%d.jpg", id))
}

// ThumbnailPath returns the path of a 96x96 JPEG thumbnail, creating it on
// first use.
func (l *Library) ThumbnailPath(id int) (string, error) {
	p := l.thumbPath(id)
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	data, err := l.Bytes(id)
	if err != nil {
		return "", err
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	sq := CropSquare(img)
	dst := image.NewRGBA(image.Rect(0, 0, thumbSide, thumbSide))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), sq, sq.Bounds(), xdraw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return "", err
	}
	if err := writeAtomic(p, buf.Bytes()); err != nil {
		return "", err
	}
	return p, nil
}

func (l *Library) find(id int) int {
	for i, it := range l.doc.Items {
		if it.ID == id {
			return i
		}
	}
	return -1
}

func (l *Library) allocID() (int, error) {
	if l.doc.NextID > MaxID {
		return 0, errors.New("cover: id space exhausted")
	}
	return l.doc.NextID, nil
}

// insert writes the file, then the index; callers hold l.mu.
func (l *Library) insert(id int, name string, data []byte, hash uint32) (Item, error) {
	it := Item{
		ID:      id,
		Hash:    hash,
		Name:    name,
		File:    fmt.Sprintf("%d.jpg", id),
		Size:    len(data),
		AddedAt: time.Now().UTC(),
	}
	if err := writeAtomic(filepath.Join(l.dir, it.File), data); err != nil {
		return Item{}, err
	}
	prevNext := l.doc.NextID
	l.doc.Items = append(l.doc.Items, it)
	if id >= l.doc.NextID && id <= MaxID {
		l.doc.NextID = id + 1
	}
	if err := l.save(); err != nil {
		l.doc.Items = l.doc.Items[:len(l.doc.Items)-1]
		l.doc.NextID = prevNext
		return Item{}, err
	}
	return it, nil
}

func (l *Library) save() error {
	data, err := json.MarshalIndent(l.doc, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filepath.Join(l.dir, indexFile), data)
}

func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}
