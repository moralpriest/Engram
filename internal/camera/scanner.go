//go:build !android

package camera

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"log"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"github.com/svanichkin/gocam"
)

type DesktopScanner struct {
	window fyne.Window
	onScan func(string)
	cancel context.CancelFunc
	mu     sync.Mutex
}

func NewScanner(win fyne.Window, onScan func(string)) *DesktopScanner {
	return &DesktopScanner{
		window: win,
		onScan: onScan,
	}
}

func (s *DesktopScanner) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	frames, err := gocam.StartStream(ctx)
	if err != nil {
		cancel()
		log.Printf("gocam.StartStream failed: %v", err)
		return fmt.Errorf("failed to start camera stream: %v", err)
	}

	imgWidget := canvas.NewImageFromImage(nil)
	imgWidget.FillMode = canvas.ImageFillContain
	imgWidget.SetMinSize(fyne.NewSize(400, 300))

	content := container.NewMax(imgWidget)
	d := dialog.NewCustom("Scan QR Code", "Close", content, s.window)

	d.SetOnClosed(func() {
		s.Stop()
	})

	go func() {
		reader := qrcode.NewQRCodeReader()
		ticker := time.NewTicker(time.Millisecond * 200) // Scan every ~5 frames
		defer ticker.Stop()

		hints := make(map[gozxing.DecodeHintType]interface{})
		hints[gozxing.DecodeHintType_TRY_HARDER] = true

		fyne.Do(func() {
			d.Show()
		})

		for {
			select {
			case <-ctx.Done():
				return
			case frame, ok := <-frames:
				if !ok {
					return
				}

				// Convert gocam.Frame to image.Image
				goImg := frameToImage(frame)

				// Update live preview (must run on Fyne thread)
				fyne.Do(func() {
					imgWidget.Image = goImg
					imgWidget.Refresh()
				})

				// Deciding whether to scan this frame
				select {
				case <-ticker.C:
					// Decode using HybridBinarizer for better camera performance
					ls := gozxing.NewLuminanceSourceFromImage(goImg)
					bmp, err := gozxing.NewBinaryBitmap(gozxing.NewHybridBinarizer(ls))
					if err != nil {
						continue
					}

					result, err := reader.Decode(bmp, hints)
					if err == nil {
						text := result.GetText()
						fyne.Do(func() {
							s.onScan(text)
							d.Hide()
							s.Stop()
						})
						return
					}
				default:
				}
			}
		}
	}()

	d.Show()
	return nil
}

func (s *DesktopScanner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

func frameToImage(f gocam.Frame) image.Image {
	if f.Width <= 0 || f.Height <= 0 || len(f.Data) != f.Width*f.Height*3 {
		return nil
	}

	img := image.NewNRGBA(image.Rect(0, 0, f.Width, f.Height))
	for y := 0; y < f.Height; y++ {
		for x := 0; x < f.Width; x++ {
			idx := (y*f.Width + x) * 3
			yVal := f.Data[idx]
			cb := f.Data[idx+1]
			cr := f.Data[idx+2]
			r, g, b := color.YCbCrToRGB(yVal, cb, cr)

			di := img.PixOffset(x, y)
			img.Pix[di+0] = r
			img.Pix[di+1] = g
			img.Pix[di+2] = b
			img.Pix[di+3] = 0xff
		}
	}
	return img
}
