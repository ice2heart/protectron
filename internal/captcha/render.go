package captcha

import (
	"bytes"
	"image/color"
	"image/png"
	"math/rand/v2"
	"sync"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"

	"github.com/ice2heart/protectron/assets"
)

// Custom renderer (see doc/plan.md, Phase 0): steambap/captcha can't take
// caller-supplied text and its charset handling is not UTF-8-safe, so we
// draw the glyphs ourselves in a similar style — per-glyph rotation, jitter
// and color, plus noise curves and dots. Visual jitter uses math/rand: the
// secret (the text) is already fixed by crypto/rand before rendering.

const (
	imgWidth  = 350
	imgHeight = 200
)

var (
	fontOnce   sync.Once
	parsedFont *truetype.Font
	fontErr    error
)

func captchaFont() (*truetype.Font, error) {
	fontOnce.Do(func() {
		parsedFont, fontErr = truetype.Parse(assets.CaptchaFont)
	})
	return parsedFont, fontErr
}

func randDarkColor() color.Color {
	return color.RGBA{
		R: uint8(rand.IntN(150)),
		G: uint8(rand.IntN(150)),
		B: uint8(rand.IntN(150)),
		A: 255,
	}
}

// render draws text into a noisy PNG.
func render(text string) ([]byte, error) {
	ttf, err := captchaFont()
	if err != nil {
		return nil, err
	}

	dc := gg.NewContext(imgWidth, imgHeight)
	dc.SetRGB(0.93, 0.93, 0.90)
	dc.Clear()

	runes := []rune(text)
	spacing := float64(imgWidth) / float64(len(runes)+1)

	for i, r := range runes {
		size := 52 + rand.Float64()*26
		face := truetype.NewFace(ttf, &truetype.Options{Size: size, Hinting: font.HintingFull})
		dc.SetFontFace(face)
		dc.SetColor(randDarkColor())

		x := spacing * float64(i+1)
		y := float64(imgHeight)/2 + (rand.Float64()-0.5)*40
		angle := (rand.Float64() - 0.5) * 0.6 // ±~17°

		dc.Push()
		dc.RotateAbout(angle, x, y)
		dc.DrawStringAnchored(string(r), x, y, 0.5, 0.5)
		dc.Pop()
	}

	for c := 0; c < 2+rand.IntN(3); c++ {
		dc.SetColor(randDarkColor())
		dc.SetLineWidth(1.5 + rand.Float64()*1.5)
		x0 := rand.Float64() * imgWidth / 4
		x1 := imgWidth - rand.Float64()*imgWidth/4
		y0 := rand.Float64() * imgHeight
		y1 := rand.Float64() * imgHeight
		cx := imgWidth/2 + (rand.Float64()-0.5)*100
		cy := rand.Float64() * imgHeight
		dc.MoveTo(x0, y0)
		dc.QuadraticTo(cx, cy, x1, y1)
		dc.Stroke()
	}

	for d := 0; d < imgWidth*imgHeight/280; d++ {
		dc.SetColor(randDarkColor())
		dc.DrawCircle(rand.Float64()*imgWidth, rand.Float64()*imgHeight, rand.Float64()*1.5)
		dc.Fill()
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, dc.Image()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
