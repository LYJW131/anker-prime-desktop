package backend

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testID = "0123456789abcdef0123456789abcdef01234567"

func TestUserIDFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if got := readUserID(dir); got != "" {
		t.Fatalf("readUserID without a file = %q", got)
	}
	if err := writeUserID(dir, testID); err != nil {
		t.Fatal(err)
	}
	if got := readUserID(dir); got != testID {
		t.Fatalf("readUserID = %q, want the written ID", got)
	}
	fi, err := os.Stat(filepath.Join(dir, userIDFile))
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode %o, want 600", perm)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("%d entries in the directory, want the file alone", len(entries))
	}

	if err := os.WriteFile(filepath.Join(dir, userIDFile), []byte("short\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readUserID(dir); got != "" {
		t.Errorf("readUserID of a malformed file = %q", got)
	}
}

func TestLoadUserIDMovesTheSecret(t *testing.T) {
	dir := t.TempDir()
	forgot := false
	got := loadUserID(dir,
		func() ([]byte, error) { return []byte(" " + testID + "\n"), nil },
		func() error { forgot = true; return nil })
	if got != testID || readUserID(dir) != testID || !forgot {
		t.Fatalf("loadUserID = %q, file %q, forgot %v", got, readUserID(dir), forgot)
	}

	// Once the file holds it, the secret store is not asked again.
	got = loadUserID(dir,
		func() ([]byte, error) { t.Fatal("asked the secret store"); return nil, nil },
		func() error { t.Fatal("forgot again"); return nil })
	if got != testID {
		t.Fatalf("loadUserID from the file = %q", got)
	}
}

func TestLoadUserIDWithoutOne(t *testing.T) {
	for name, legacy := range map[string]func() ([]byte, error){
		"no secret":   func() ([]byte, error) { return nil, errors.New("not found") },
		"invalid one": func() ([]byte, error) { return []byte(strings.Repeat("x", 39)), nil },
		"unsupported": func() ([]byte, error) { return nil, errors.ErrUnsupported },
	} {
		dir := t.TempDir()
		got := loadUserID(dir, legacy, func() error { t.Errorf("%s: forgot", name); return nil })
		if got != "" || readUserID(dir) != "" {
			t.Errorf("%s: loadUserID = %q, file %q", name, got, readUserID(dir))
		}
	}
}

func TestNewUserID(t *testing.T) {
	a, b := newUserID(), newUserID()
	if !validUserID(a) || a == b {
		t.Fatalf("newUserID = %q, %q", a, b)
	}
}
