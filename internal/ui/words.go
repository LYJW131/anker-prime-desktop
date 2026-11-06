package ui

import (
	"errors"
	"fmt"

	"github.com/LYJW131/anker-prime-desktop/internal/ble"
	"github.com/LYJW131/anker-prime-desktop/internal/link"
	"github.com/LYJW131/anker-prime-desktop/internal/store"
	"github.com/LYJW131/anker-prime-desktop/locales"
)

// ErrText says an error of the links, the backend or the store in the
// app's language; any other error says its own text.
func ErrText(err error) string {
	if err == nil {
		return ""
	}
	var hs *link.HandshakeError
	var rej *link.CoverRejectedError
	switch {
	case errors.As(err, &hs):
		return locales.ErrHandshake(ErrText(hs.Err))
	case errors.As(err, &rej):
		return locales.ErrCoverRejected(fmt.Sprintf("0x%04X", rej.Command), fmt.Sprintf("0x%02X", rej.Result))
	case errors.Is(err, link.ErrManual):
		return locales.ErrManual()
	case errors.Is(err, link.ErrNoUserID):
		return locales.ErrNoUserID()
	case errors.Is(err, link.ErrStalled):
		return locales.ErrStalled()
	case errors.Is(err, link.ErrNoResponse):
		return locales.ErrNoResponse()
	case errors.Is(err, link.ErrNotConnected):
		return locales.ErrNotConnected()
	case errors.Is(err, link.ErrCoverNoData):
		return locales.ErrCoverNoData()
	case errors.Is(err, link.ErrCoverNotSwitched):
		return locales.ErrCoverNotSwitched()
	case errors.Is(err, ble.ErrDisconnected):
		return locales.ErrDisconnected()
	case errors.Is(err, ble.ErrNotPoweredOn):
		return locales.ErrBluetoothOff()
	case errors.Is(err, ble.ErrUnauthorized):
		return locales.ErrBluetoothDenied()
	case errors.Is(err, ble.ErrTimeout):
		return locales.ErrTimeout()
	case errors.Is(err, store.ErrUserIDFormat):
		return locales.ErrUserIDFormat()
	case errors.Is(err, store.ErrNoDataDir):
		return locales.ErrNoDataDir()
	}
	return err.Error()
}

// NoticeText says a toast in the app's language.
func NoticeText(n *store.Notice) string {
	if n == nil {
		return ""
	}
	var what string
	switch n.Kind {
	case store.NoticeCoverShown:
		what = locales.NoticeCoverShown(n.Subject)
	case store.NoticeCoverFailed:
		what = locales.NoticeCoverFailed()
	case store.NoticeCoverAdded:
		what = locales.NoticeCoverAdded(n.Subject)
	case store.NoticeCoverAddFailed:
		what = locales.NoticeCoverAddFailed()
	case store.NoticeCoverRemoveFailed:
		what = locales.NoticeCoverRemoveFailed()
	case store.NoticeCoverRenameFailed:
		what = locales.NoticeCoverRenameFailed()
	case store.NoticeBluetoothUnavailable:
		what = locales.NoticeBluetooth()
	case store.NoticeLibraryUnavailable:
		what = locales.NoticeLibrary()
	case store.NoticeSettingsNotSaved:
		what = locales.NoticeSettings()
	case store.NoticeDeviceAdded:
		what = locales.NoticeDeviceAdded(n.Subject)
	case store.NoticeUserIDSaved:
		what = locales.NoticeUserIDSaved()
	}
	if n.Err == nil {
		return what
	}
	if what == "" {
		return ErrText(n.Err)
	}
	return locales.ReasonJoin(what, ErrText(n.Err))
}
