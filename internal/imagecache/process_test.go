package imagecache

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"testing"
)

func TestProcessShrinksAndReencodes(t *testing.T) {
	src := jpegBytes(t, noisy(1000, 1500, 1), 95)
	enc, err := process(src, 600, 82)
	if err != nil || enc.Format != FormatJPEG || enc.Width != 600 || enc.Height != 900 || enc.Quality != 82 || len(enc.Data) >= len(src) {
		t.Fatalf("%+v %v", enc.Format, err)
	}
	small, _ := process(jpegBytes(t, noisy(200, 300, 2), 90), 600, 82)
	if small.Width != 200 {
		t.Fatal("never scaled up")
	}
}

func TestTransparencyKeepsPNGAndOpaquePNGBecomesJPEG(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 40, 40))
	img.Set(1, 1, color.NRGBA{255, 0, 0, 128})
	enc, err := process(pngBytes(t, img), 600, 82)
	if err != nil || enc.Format != FormatPNG || enc.Quality != 0 {
		t.Fatal(enc.Format, err)
	}
	enc, _ = process(pngBytes(t, noisy(40, 40, 3)), 600, 82)
	if enc.Format != FormatJPEG {
		t.Fatal(enc.Format)
	}
}

func TestWebPIsRead(t *testing.T) {
	// a 1×1 lossless WebP
	webp := []byte("RIFF\x1a\x00\x00\x00WEBPVP8L\x0d\x00\x00\x00\x2f\x00\x00\x00\x10\x07\x10\x11\x11\x88\x88\xfe\x07\x00")
	enc, err := process(webp, 600, 82)
	if err != nil || enc.Width != 1 {
		t.Fatal(enc, err)
	}
}

func TestEmbeddedMetadataIsDropped(t *testing.T) {
	src := jpegBytes(t, noisy(64, 64, 4), 90)
	exif := append([]byte{0xFF, 0xE1, 0x00, 0x10}, []byte("Exif\x00\x00GPS-SECRET")...)
	withExif := append(append(append([]byte{}, src[:2]...), exif...), src[2:]...)
	if !contains(withExif, "GPS-SECRET") {
		t.Fatal("fixture")
	}
	enc, err := process(withExif, 600, 82)
	if err != nil || contains(enc.Data, "GPS-SECRET") || contains(enc.Data, "Exif") {
		t.Fatal("metadata kept", err)
	}
}

func TestDecodeBombsAndGarbageAreRefused(t *testing.T) {
	if _, err := process([]byte("<html>not an image</html>"), 600, 82); !errors.Is(err, ErrNotImage) {
		t.Fatal(err)
	}
	bomb := jpegBytes(t, noisy(16, 16, 5), 80)
	i := bytes.Index(bomb, []byte{0xFF, 0xC0})
	if i < 0 {
		t.Fatal("no SOF0")
	}
	bomb[i+5], bomb[i+6], bomb[i+7], bomb[i+8] = 0xEA, 0x60, 0xEA, 0x60 // 60000 × 60000
	if _, err := process(bomb, 600, 82); !errors.Is(err, ErrTooLarge) {
		t.Fatal(err)
	}
	trunc := jpegBytes(t, noisy(64, 64, 6), 80)
	if _, err := process(trunc[:len(trunc)/3], 600, 82); !errors.Is(err, ErrNotImage) {
		t.Fatal(err)
	}
}
