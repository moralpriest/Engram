package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"github.com/civilware/tela/logger"
	"github.com/creachadair/jrpc2"
	"github.com/creachadair/jrpc2/channel"
	"github.com/deroproject/derohe/rpc"
	"github.com/gorilla/websocket"
	"github.com/hypergnomon/hypergnomon/rpc/rwc"
)

const VILLAGER_SCID = "f0b29081c1ed35fe942cb3402cd9d7bf0cf27639201bbc96223bdc99c4c6aa9f"

var villagerPalette = map[byte]color.Color{
	'0': color.RGBA{0xFF, 0x99, 0x99, 0xFF}, '1': color.RGBA{0xFF, 0x66, 0x66, 0xFF}, '2': color.RGBA{0xFF, 0x00, 0x00, 0xFF}, '3': color.RGBA{0x80, 0x00, 0x00, 0xFF},
	'4': color.RGBA{0xFF, 0xA8, 0x99, 0xFF}, '5': color.RGBA{0xFF, 0x8C, 0x66, 0xFF}, '6': color.RGBA{0xFF, 0x45, 0x00, 0xFF}, '7': color.RGBA{0x80, 0x22, 0x00, 0xFF},
	'8': color.RGBA{0xFF, 0xC7, 0x99, 0xFF}, '9': color.RGBA{0xFF, 0xB2, 0x66, 0xFF}, 'A': color.RGBA{0xFF, 0x8C, 0x00, 0xFF}, 'B': color.RGBA{0x80, 0x46, 0x00, 0xFF},
	'C': color.RGBA{0xFF, 0xE0, 0x99, 0xFF}, 'D': color.RGBA{0xFF, 0xD8, 0x66, 0xFF}, 'E': color.RGBA{0xFF, 0xAA, 0x00, 0xFF}, 'F': color.RGBA{0x5C, 0x40, 0x33, 0xFF},
	'G': color.RGBA{0xFF, 0xFF, 0x99, 0xFF}, 'H': color.RGBA{0xFF, 0xFF, 0x66, 0xFF}, 'I': color.RGBA{0xFF, 0xFF, 0x00, 0xFF}, 'J': color.RGBA{0xFF, 0xD7, 0x00, 0xFF},
	'K': color.RGBA{0xCF, 0xFF, 0x99, 0xFF}, 'L': color.RGBA{0xBF, 0xFF, 0x66, 0xFF}, 'M': color.RGBA{0x80, 0xFF, 0x00, 0xFF}, 'N': color.RGBA{0x40, 0x80, 0x00, 0xFF},
	'O': color.RGBA{0x99, 0xFF, 0x99, 0xFF}, 'P': color.RGBA{0x66, 0xFF, 0x66, 0xFF}, 'Q': color.RGBA{0x00, 0xFF, 0x00, 0xFF}, 'R': color.RGBA{0x00, 0x80, 0x00, 0xFF},
	'S': color.RGBA{0x99, 0xFF, 0xCF, 0xFF}, 'T': color.RGBA{0x66, 0xFF, 0xBF, 0xFF}, 'U': color.RGBA{0x00, 0xFF, 0x80, 0xFF}, 'V': color.RGBA{0x00, 0x80, 0x40, 0xFF},
	'W': color.RGBA{0x99, 0xFF, 0xFF, 0xFF}, 'X': color.RGBA{0x66, 0xFF, 0xFF, 0xFF}, 'Y': color.RGBA{0x00, 0xFF, 0xFF, 0xFF}, 'Z': color.RGBA{0x00, 0x80, 0x80, 0xFF},
	'a': color.RGBA{0x99, 0xCF, 0xFF, 0xFF}, 'b': color.RGBA{0x66, 0xBF, 0xFF, 0xFF}, 'c': color.RGBA{0x00, 0x80, 0xFF, 0xFF}, 'd': color.RGBA{0x00, 0x40, 0x80, 0xFF},
	'e': color.RGBA{0x99, 0x99, 0xFF, 0xFF}, 'f': color.RGBA{0x66, 0x66, 0xFF, 0xFF}, 'g': color.RGBA{0x00, 0x00, 0xFF, 0xFF}, 'h': color.RGBA{0x00, 0x00, 0x80, 0xFF},
	'i': color.RGBA{0xCF, 0x99, 0xFF, 0xFF}, 'j': color.RGBA{0xBF, 0x66, 0xFF, 0xFF}, 'k': color.RGBA{0x80, 0x00, 0xFF, 0xFF}, 'l': color.RGBA{0x40, 0x00, 0x80, 0xFF},
	'm': color.RGBA{0xFF, 0x99, 0xFF, 0xFF}, 'n': color.RGBA{0xFF, 0x66, 0xFF, 0xFF}, 'o': color.RGBA{0xFF, 0x00, 0xFF, 0xFF}, 'p': color.RGBA{0x80, 0x00, 0x80, 0xFF},
	'q': color.RGBA{0xFF, 0x99, 0xC7, 0xFF}, 'r': color.RGBA{0xFF, 0x66, 0xB2, 0xFF}, 's': color.RGBA{0xFF, 0x00, 0x80, 0xFF}, 't': color.RGBA{0x80, 0x00, 0x40, 0xFF},
	'u': color.RGBA{0xFF, 0xFF, 0xFF, 0xFF}, 'v': color.RGBA{0xB4, 0xB4, 0xB4, 0xFF}, 'w': color.RGBA{0x84, 0x84, 0x84, 0xFF}, 'x': color.RGBA{0x43, 0x43, 0x43, 0xFF},
	'y': color.RGBA{0x00, 0x00, 0x00, 0xFF}, 'z': color.Transparent,
}

func villagerSimpleHash(str string) uint32 {
	h := uint32(1779033703 ^ len(str))
	for i := 0; i < len(str); i++ {
		h = uint32(int32(h^uint32(str[i])) * int32(-862048943))
		h = (h << 13) | (h >> 19)
	}
	return h
}

func fetchVillagerPixels(address string) (string, error) {
	if session.Offline {
		return "", fmt.Errorf("wallet is offline")
	}

	if rpc_client.RPC == nil {
		ws, _, err := websocket.DefaultDialer.Dial("ws://"+session.Daemon+"/ws", nil)
		if err != nil {
			return "", err
		}
		rpc_client.WS = ws
		input_output := rwc.New(rpc_client.WS)
		rpc_client.RPC = jrpc2.NewClient(channel.RawJSON(input_output, input_output), nil)
	}

	params := rpc.GetSC_Params{
		SCID:       VILLAGER_SCID,
		KeysString: []string{"avatar_" + address},
	}

	var result rpc.GetSC_Result
	err := rpc_client.RPC.CallResult(context.Background(), "DERO.GetSC", params, &result)
	if err != nil {
		return "", err
	}

	if len(result.ValuesString) > 0 {
		avatarHex := result.ValuesString[0]
		// The daemon answers "NOT AVAILABLE err: ..." when the key does not
		// exist. Hex-decoding that placeholder produced noisy per-pulse error
		// logs, so treat it the same as an empty avatar.
		if avatarHex == "" || strings.HasPrefix(avatarHex, "NOT AVAILABLE") {
			return "", fmt.Errorf("no avatar stored for address")
		}
		decoded, err := hex.DecodeString(avatarHex)
		if err != nil {
			return "", fmt.Errorf("avatar value is not valid hex")
		}
		return string(decoded), nil
	}

	return "", fmt.Errorf("no villager found")
}

func renderVillager(address string, pixelStr string) *canvas.Image {
	if len(pixelStr) != 576 {
		return nil
	}

	uniquePart := address
	if strings.HasPrefix(address, "dero1") {
		uniquePart = address[5:]
	}

	bgSeed := villagerSimpleHash(uniquePart + "BACKGROUND")
	frameSeed := villagerSimpleHash(uniquePart + "FRAME")

	const renderSize = 1024
	img := image.NewRGBA(image.Rect(0, 0, renderSize, renderSize))

	if session.VillagerBackground {
		drawBackground(img, uniquePart, bgSeed)
		drawStars(img, uniquePart, renderSize)
		drawFrame(img, uniquePart, frameSeed, renderSize)
	}

	// Render pixels (Avatar)
	innerSize := 757
	cellSize := innerSize / 24
	border := (renderSize - (cellSize * 24)) / 2

	idx := 0
	for x := 0; x < 24; x++ {
		for y := 0; y < 24; y++ {
			ch := pixelStr[idx]
			idx++
			c, ok := villagerPalette[ch]
			if !ok || c == color.Transparent {
				continue
			}

			rect := image.Rect(
				border+x*cellSize,
				border+y*cellSize,
				border+(x+1)*cellSize,
				border+(y+1)*cellSize,
			)
			draw.Draw(img, rect, &image.Uniform{c}, image.Point{}, draw.Over)
		}
	}

	resImage := canvas.NewImageFromImage(img)
	resImage.FillMode = canvas.ImageFillContain
	resImage.ScaleMode = canvas.ImageScaleSmooth
	return resImage
}

func drawBackground(img *image.RGBA, uniquePart string, bgSeed uint32) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	cx, cy := float64(w)/2, float64(h)/2

	stops := []struct {
		pos   float64
		hue   float64
		light float64
	}{
		{0.0, float64(bgSeed % 360), 0.45},
		{0.2, float64((bgSeed + 45) % 360), 0.38},
		{0.4, float64((bgSeed + 90) % 360), 0.30},
		{0.6, float64((bgSeed + 135) % 360), 0.22},
		{0.8, float64((bgSeed + 170) % 360), 0.16},
		{1.0, float64((bgSeed + 200) % 360), 0.12},
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			dx := float64(x) - cx
			dy := float64(y) - cy
			dist := math.Sqrt(dx*dx+dy*dy) / (float64(w) * 0.75)
			if dist > 1 {
				dist = 1
			}

			var hue, light float64
			for i := 0; i < len(stops)-1; i++ {
				if dist >= stops[i].pos && dist <= stops[i+1].pos {
					t := (dist - stops[i].pos) / (stops[i+1].pos - stops[i].pos)
					hue = stops[i].hue + t*(stops[i+1].hue-stops[i].hue)
					light = stops[i].light + t*(stops[i+1].light-stops[i].light)
					break
				}
			}

			img.Set(x, y, hslToRGB(hue, 1.0, light))
		}
	}
}

func drawStars(img *image.RGBA, uniquePart string, renderSize int) {
	for i := 0; i < renderSize; i++ {
		x := int(villagerSimpleHash(uniquePart+fmt.Sprint(i)) % uint32(renderSize))
		y := int(villagerSimpleHash(uniquePart+fmt.Sprint(i+7777)) % uint32(renderSize))
		b := 40 + (villagerSimpleHash(uniquePart+fmt.Sprint(i+99999)) % 60)
		starColor := hslToRGB(70, 0.4, float64(b)/100.0)

		img.Set(x, y, starColor)
		if villagerSimpleHash(uniquePart+fmt.Sprint(i+22222))%20 > 15 {
			img.Set(x+1, y, starColor)
			img.Set(x-1, y, starColor)
			img.Set(x, y+1, starColor)
			img.Set(x, y-1, starColor)
		}
	}
}

func drawFrame(img *image.RGBA, uniquePart string, frameSeed uint32, renderSize int) {
	shapeType := frameSeed % 5
	hueBase := float64(frameSeed % 360)
	cx, cy := float64(renderSize)/2, float64(renderSize)/2
	inner := float64(renderSize) * 0.74
	border := (float64(renderSize) - inner) / 2

	if shapeType == 2 { // Nebula rings
		ringCount := 4 + int(villagerSimpleHash(uniquePart+"GLITCH_COUNT")%6)
		for i := ringCount; i >= 1; i-- {
			r := inner/2 + border*(0.2+float64(villagerSimpleHash(uniquePart+fmt.Sprint(i))%80)/100.0)*float64(i)/float64(ringCount)
			hueShift := math.Mod(hueBase+float64(i)*30, 360)
			ringColor := hslToRGB(hueShift, 0.95, 0.74)
			ringColor.A = 160

			startAngle := float64(villagerSimpleHash(uniquePart+fmt.Sprint(i+6000))%360) * math.Pi / 180.0
			arcLength := (0.5 + float64(villagerSimpleHash(uniquePart+fmt.Sprint(i+7000))%50)/100.0) * math.Pi * 2.0

			thickness := 2 + int(villagerSimpleHash(uniquePart+fmt.Sprint(i+2000))%4)
			for a := startAngle; a < startAngle+arcLength; a += 0.005 {
				if int(a*100)%2 == 0 {
					for t := 0; t < thickness; t++ {
						tr := r + float64(t)
						px := cx + math.Cos(a)*tr
						py := cy + math.Sin(a)*tr
						img.Set(int(px), int(py), ringColor)
					}
				}
			}
		}
	} else {
		shardCount := 12 + int(villagerSimpleHash(uniquePart+"SHARDS")%12)
		for i := 0; i < shardCount; i++ {
			a := float64(i) / float64(shardCount) * math.Pi * 2
			r := inner/2 + border*0.6
			h := math.Mod(hueBase+float64(i*15), 360)
			c := hslToHexColor(h, 0.9, 0.65)
			c.A = 120

			px := cx + math.Cos(a)*r
			py := cy + math.Sin(a)*r

			shardSize := 4 + int(villagerSimpleHash(uniquePart+fmt.Sprint(i))%8)
			for dx := -shardSize; dx <= shardSize; dx++ {
				for dy := -shardSize; dy <= shardSize; dy++ {
					if math.Abs(float64(dx))+math.Abs(float64(dy)) < float64(shardSize)*1.5 {
						img.Set(int(px)+dx, int(py)+dy, c)
					}
				}
			}
		}
	}
}

func hslToHexColor(h, s, l float64) color.RGBA {
	return hslToRGB(h, s, l)
}

func hslToRGB(h, s, l float64) color.RGBA {
	var r, g, b float64
	if s == 0 {
		r, g, b = l, l, l
	} else {
		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}
		p := 2*l - q
		r = hueToRGB(p, q, h/360+1.0/3.0)
		g = hueToRGB(p, q, h/360)
		b = hueToRGB(p, q, h/360-1.0/3.0)
	}
	return color.RGBA{uint8(r * 255), uint8(g * 255), uint8(b * 255), 255}
}

func hueToRGB(p, q, t float64) float64 {
	if t < 0 {
		t += 1
	}
	if t > 1 {
		t -= 1
	}
	if t < 1.0/6.0 {
		return p + (q-p)*6*t
	}
	if t < 1.0/2.0 {
		return q
	}
	if t < 2.0/3.0 {
		return p + (q-p)*(2.0/3.0-t)*6
	}
	return p
}

func updateVillagerAvatar() {
	if engram.Disk == nil {
		return
	}

	address := engram.Disk.GetAddress().String()
	if session.VillagerAddress == address && res.villager != nil {
		return
	}

	pixels, err := fetchVillagerPixels(address)
	if err != nil {
		logger.Printf("[Villager] No avatar found for %s: %v", address, err)
		res.villagerMu.Lock()
		res.villager = nil
		res.villagerMu.Unlock()
		return
	}

	// Double check we're still on the same wallet before updating state
	if engram.Disk == nil || engram.Disk.GetAddress().String() != address {
		return
	}

	session.VillagerPixels = pixels
	villagerImg := renderVillager(address, pixels)

	res.villagerMu.Lock()
	res.villager = villagerImg
	session.VillagerAddress = address
	res.villagerMu.Unlock()

	if session.Dashboard == "main" {
		fyne.Do(func() {
			if session.Window != nil && session.Domain == "app.wallet" {
				RefreshVillagerLogo()
			}
		})
	}
}
