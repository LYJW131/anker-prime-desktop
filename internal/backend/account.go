package backend

import (
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// userIDFile holds the Anker account ID in the user data directory,
// readable by the user only. Not the Keychain: an app without a team ID
// gets a Keychain partition of its build's code hash, so every rebuild and
// every update asked for the login password again.
const userIDFile = "anker-user-id"

// validUserID reports whether id has the account ID's form: 40 printable
// ASCII characters.
func validUserID(id string) bool {
	if len(id) != 40 {
		return false
	}
	for _, r := range id {
		if r > 0x7e || r < 0x21 {
			return false
		}
	}
	return true
}

// newUserID returns a random ID of the account ID's form: 40 hex digits.
func newUserID() string {
	b := make([]byte, 20)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// readUserID returns the ID in dir, or "" without a valid one.
func readUserID(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, userIDFile))
	if err != nil {
		return ""
	}
	if id := strings.TrimSpace(string(b)); validUserID(id) {
		return id
	}
	return ""
}

// writeUserID replaces the ID in dir, never leaving a partial file.
func writeUserID(dir, id string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+userIDFile+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err := f.Chmod(0o600); err != nil {
		f.Close()
		return err
	}
	if _, err := f.WriteString(id + "\n"); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), filepath.Join(dir, userIDFile))
}

// loadUserID returns the ID in dir. Without one it moves the ID the
// earlier versions kept in the secret store into dir, which may ask for
// the Keychain once more.
func loadUserID(dir string, legacy func() ([]byte, error), forget func() error) string {
	if id := readUserID(dir); id != "" {
		return id
	}
	b, err := legacy()
	if err != nil {
		return ""
	}
	id := strings.TrimSpace(string(b))
	if !validUserID(id) {
		return ""
	}
	if writeUserID(dir, id) == nil {
		_ = forget()
	}
	return id
}
