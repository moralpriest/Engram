//go:build android

package camera

import (
	"errors"

	"fyne.io/fyne/v2"
)

type AndroidScanner struct{}

func NewScanner(_ fyne.Window, _ func(string)) *AndroidScanner {
	return &AndroidScanner{}
}

func (s *AndroidScanner) Start() error {
	return errors.New("QR scanning not available on mobile")
}

func (s *AndroidScanner) Stop() {}
