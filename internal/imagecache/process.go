package imagecache

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // WebP decoding
)

// Decode limits: refuse decode bombs before allocating pixels.
const (
	maxPixels = 40_000_000
	maxSide   = 12_000
)

// Formats of stored files.
const (
	FormatJPEG = "jpeg"
	FormatPNG  = "png"
)

// ErrNotImage is a body that is not a readable JPEG, PNG or WebP image.
var ErrNotImage = errors.New("not a JPEG, PNG or WebP image")

// ErrTooLarge is an image whose source or pixel size is over the limits.
var ErrTooLarge = errors.New("image is too large")

// Encoded is a re-encoded image.
type Encoded struct {
	Data    []byte
	Format  string
	Width   int
	Height  int
	Quality int // 0 for PNG
}

// decode reads an image, refusing it before the full decode when its pixel
// size is over the limits.
func decode(data []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, ErrNotImage
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxSide || cfg.Height > maxSide || cfg.Width*cfg.Height > maxPixels {
		return nil, fmt.Errorf("%w: %d×%d pixels", ErrTooLarge, cfg.Width, cfg.Height)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, ErrNotImage
	}
	return img, nil
}

// encode scales img down to at most width (never up) and stores it as JPEG
// at quality, or as PNG when it has transparency. Re-encoding drops EXIF and
// every other embedded metadata.
func encode(img image.Image, width, quality int) (Encoded, error) {
	img = shrink(img, width)
	b := img.Bounds()
	var buf bytes.Buffer
	if !opaque(img) {
		if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&buf, img); err != nil {
			return Encoded{}, err
		}
		return Encoded{Data: buf.Bytes(), Format: FormatPNG, Width: b.Dx(), Height: b.Dy()}, nil
	}
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return Encoded{}, err
	}
	return Encoded{Data: buf.Bytes(), Format: FormatJPEG, Width: b.Dx(), Height: b.Dy(), Quality: quality}, nil
}

// process decodes source bytes and re-encodes them.
func process(data []byte, width, quality int) (Encoded, error) {
	img, err := decode(data)
	if err != nil {
		return Encoded{}, err
	}
	return encode(img, width, quality)
}

// shrink resamples img to width with Catmull-Rom, keeping the aspect ratio.
func shrink(img image.Image, width int) image.Image {
	b := img.Bounds()
	if width <= 0 || b.Dx() <= width {
		return img
	}
	h := max(1, b.Dy()*width/b.Dx())
	dst := image.NewRGBA(image.Rect(0, 0, width, h))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Src, nil)
	return dst
}

// opaque reports whether every pixel is fully opaque.
func opaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	return true
}
