//go:build !darwin

package app

// scrollbarsAlways reports false: the app is built for macOS; elsewhere
// thumbs show while a list moves.
func scrollbarsAlways() bool { return false }
