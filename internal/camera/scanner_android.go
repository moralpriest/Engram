//go:build android

package camera

import (
	"context"
	"fmt"
	"image"
	"log"
	"sync"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

/*
#include <jni.h>
#include <stdlib.h>

// Functions implemented in the Fyne vendor android.c
void startCamera(JNIEnv* env);
void stopCamera(JNIEnv* env);

// Polling functions implemented in jni_callbacks_android.c
int pollCameraFrame(unsigned char* outBuf, int outBufSize, int* outWidth, int* outHeight);
int pollCameraError(char* outBuf, int outBufSize);
void cleanupCameraBuffers();
*/
import "C"

// AndroidScanner implements in-app QR scanning using Camera2 + gozxing
type AndroidScanner struct {
	window fyne.Window
	onScan func(string)
	cancel context.CancelFunc
	mu     sync.Mutex
}

func NewScanner(win fyne.Window, onScan func(string)) *AndroidScanner {
	return &AndroidScanner{
		window: win,
		onScan: onScan,
	}
}

// placeholderImage creates a dark placeholder for the viewfinder
func placeholderImage(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i+0] = 30
		img.Pix[i+1] = 30
		img.Pix[i+2] = 30
		img.Pix[i+3] = 0xFF
	}
	return img
}

func (s *AndroidScanner) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	// Clean any leftover state
	C.cleanupCameraBuffers()

	// Build the viewfinder UI on the Fyne UI thread (current thread)
	imgWidget := canvas.NewImageFromImage(placeholderImage(240, 320))
	imgWidget.FillMode = canvas.ImageFillContain
	imgWidget.SetMinSize(fyne.NewSize(240, 320))

	content := container.NewMax(imgWidget)

	d := dialog.NewCustom("Scan QR Code", "Close", content, s.window)
	d.SetOnClosed(func() {
		s.Stop()
	})
	d.Show()

	// Start camera via JNI (posts to Android UI thread)
	go func() {
		err := driver.RunNative(func(ctx2 any) error {
			ac, ok := ctx2.(*driver.AndroidContext)
			if !ok {
				return fmt.Errorf("not running on Android")
			}
			env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
			C.startCamera(env)
			return nil
		})
		if err != nil {
			log.Printf("QR: Failed to start camera: %v", err)
		}
	}()

	// Frame polling goroutine — polls the C shared buffer, never called from Java
	go s.pollFrames(ctx, d, imgWidget)

	return nil
}

func (s *AndroidScanner) pollFrames(ctx context.Context, d dialog.Dialog, imgWidget *canvas.Image) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("QR: panic in pollFrames: %v", r)
		}
	}()

	reader := qrcode.NewQRCodeReader()
	hints := make(map[gozxing.DecodeHintType]interface{})
	hints[gozxing.DecodeHintType_TRY_HARDER] = true

	// Pre-allocate buffers for polling
	const maxFrameSize = 1920 * 1080 // supports up to 1080p
	frameBuf := make([]byte, maxFrameSize)
	errBuf := make([]byte, 256)

	pollTicker := time.NewTicker(50 * time.Millisecond) // ~20fps poll rate
	defer pollTicker.Stop()

	decodeTicker := time.NewTicker(200 * time.Millisecond)
	defer decodeTicker.Stop()

	shouldDecode := false

	for {
		select {
		case <-ctx.Done():
			C.cleanupCameraBuffers()
			fyne.Do(func() {
				d.Hide()
			})
			return

		case <-decodeTicker.C:
			shouldDecode = true

		case <-pollTicker.C:
			// Check for camera errors
			if C.pollCameraError((*C.char)(unsafe.Pointer(&errBuf[0])), C.int(len(errBuf))) != 0 {
				errMsg := C.GoString((*C.char)(unsafe.Pointer(&errBuf[0])))
				log.Printf("QR: Camera error: %s", errMsg)
				C.cleanupCameraBuffers()
				fyne.Do(func() {
					d.Hide()
					dialog.ShowError(fmt.Errorf("camera: %s", errMsg), s.window)
				})
				s.stopInternal()
				return
			}

			// Poll for new frame
			var w, h C.int
			got := C.pollCameraFrame(
				(*C.uchar)(unsafe.Pointer(&frameBuf[0])),
				C.int(maxFrameSize),
				&w, &h,
			)
			if got == 0 {
				continue
			}

			fw := int(w)
			fh := int(h)
			if fw <= 0 || fh <= 0 || fw*fh > maxFrameSize {
				continue
			}

			// Copy luminance data for this frame
			lum := make([]byte, fw*fh)
			copy(lum, frameBuf[:fw*fh])

			// Rotate 90° CW for portrait mode (camera sensor is landscape)
			rotLum := rotateLuminance90CW(lum, fw, fh)
			rotW := fh // after 90° CW rotation, width and height swap
			rotH := fw

			// Update viewfinder with rotated image
			goImg := luminanceToImage(rotLum, rotW, rotH)
			fyne.Do(func() {
				imgWidget.Image = goImg
				imgWidget.Refresh()
			})

			// QR decode attempt on rotated data
			if shouldDecode {
				shouldDecode = false

				ls := newYPlaneLuminanceSource(rotLum, rotW, rotH)
				bmp, err := gozxing.NewBinaryBitmap(gozxing.NewHybridBinarizer(ls))
				if err != nil {
					continue
				}
				result, err := reader.Decode(bmp, hints)
				if err == nil {
					text := result.GetText()
					log.Printf("QR: Decoded: %s", text)
					C.cleanupCameraBuffers()
					s.stopInternal()
					fyne.Do(func() {
						d.Hide()
						s.onScan(text)
					})
					return
				}
			}
		}
	}
}

// stopInternal cancels without locking (called from within locked contexts)
func (s *AndroidScanner) stopInternal() {
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}

	go func() {
		_ = driver.RunNative(func(ctx2 any) error {
			ac, ok := ctx2.(*driver.AndroidContext)
			if !ok {
				return nil
			}
			env := (*C.JNIEnv)(unsafe.Pointer(ac.Env))
			C.stopCamera(env)
			return nil
		})
	}()
}

func (s *AndroidScanner) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopInternal()
}

// rotateLuminance90CW rotates a luminance buffer 90° clockwise.
// Input: width x height landscape frame. Output: height x width portrait frame.
func rotateLuminance90CW(src []byte, w, h int) []byte {
	dst := make([]byte, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dst[x*h+(h-1-y)] = src[y*w+x]
		}
	}
	return dst
}

// luminanceToImage converts a Y-plane byte array to a grayscale image.Image
func luminanceToImage(lum []byte, width, height int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for i := 0; i < width*height && i < len(lum); i++ {
		y := lum[i]
		di := i * 4
		img.Pix[di+0] = y
		img.Pix[di+1] = y
		img.Pix[di+2] = y
		img.Pix[di+3] = 0xFF
	}
	return img
}

// yPlaneLuminanceSource implements gozxing.LuminanceSource using raw Y-plane data.
type yPlaneLuminanceSource struct {
	gozxing.LuminanceSourceBase
	data []byte
}

func newYPlaneLuminanceSource(data []byte, width, height int) *yPlaneLuminanceSource {
	return &yPlaneLuminanceSource{
		LuminanceSourceBase: gozxing.LuminanceSourceBase{Width: width, Height: height},
		data:                data,
	}
}

func (s *yPlaneLuminanceSource) GetRow(y int, row []byte) ([]byte, error) {
	if y < 0 || y >= s.Height {
		return nil, fmt.Errorf("row %d out of range [0,%d)", y, s.Height)
	}
	offset := y * s.Width
	if len(row) < s.Width {
		row = make([]byte, s.Width)
	}
	copy(row, s.data[offset:offset+s.Width])
	return row, nil
}

func (s *yPlaneLuminanceSource) GetMatrix() []byte {
	return s.data
}

func (s *yPlaneLuminanceSource) Invert() gozxing.LuminanceSource {
	return gozxing.LuminanceSourceInvert(s)
}

func (s *yPlaneLuminanceSource) String() string {
	return gozxing.LuminanceSourceString(s)
}
