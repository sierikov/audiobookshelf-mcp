package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// gradient is a w×h picture with something in it for an encoder to work on.
func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 255 / w), G: uint8(y * 255 / h), B: uint8((x + y) % 256), A: 255}) //nolint:gosec // every value is below 256
		}
	}
	return img
}

// noise is a w×h picture of pseudo-random pixels, which no encoder makes small.
func noise(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	seed := uint32(1)
	for i := range img.Pix {
		seed ^= seed << 13
		seed ^= seed >> 17
		seed ^= seed << 5
		img.Pix[i] = uint8(seed & 0xff)
	}
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 0xff
	}
	return img
}

func jpegOf(t *testing.T, img image.Image) abs.Image {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return abs.Image{Data: buf.Bytes(), MIMEType: "image/jpeg"}
}

func pngOf(t *testing.T, img image.Image) abs.Image {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return abs.Image{Data: buf.Bytes(), MIMEType: "image/png"}
}

func gifOf(t *testing.T, img image.Image) abs.Image {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, gif.Encode(&buf, img, nil))
	return abs.Image{Data: buf.Bytes(), MIMEType: "image/gif"}
}

// previewJPEG checks that img is a JPEG, by decoding it as one, and returns it.
func previewJPEG(t *testing.T, img abs.Image) image.Image {
	t.Helper()
	require.Equal(t, "image/jpeg", img.MIMEType)
	decoded, err := jpeg.Decode(bytes.NewReader(img.Data))
	require.NoError(t, err)
	return decoded
}

// rgbAt is the colour of a pixel, as 8-bit red, green and blue.
func rgbAt(img image.Image, x, y int) (r, g, b uint8) {
	c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
	return c.R, c.G, c.B
}

// bmp is a file in a format nothing registers a decoder for, so it can't be
// looked at: a BMP header, then zeros up to size bytes.
func bmp(size int) abs.Image {
	data := make([]byte, size)
	copy(data, "BM")
	return abs.Image{Data: data, MIMEType: "image/bmp"}
}

// The WebP fixtures are the bytes cwebp 1.6.0 wrote from PNGs made for the
// purpose: golang.org/x/image reads WebP but has no encoder to make it with.
// Each is a flat picture in two colours, which WebP makes tiny.

// webpLossyHex is a 200×300 lossy WebP (VP8, the kind FantLab serves), 290
// bytes: the left half blue, the right half red.
//
//	cwebp -q 80 small.png -o small.webp
const webpLossyHex = "524946461a01000057454250565038200e0100001015009d012ac8002c013e6d" +
	"369949242322a122a800800d89676ee17551118007e206980fc40fd80a501f80" +
	"15dffb8000117563585a672beaefab1e6d4164746387c211764746387c211764" +
	"746387c211764746387c211764746387c211764746387c211764746387c21176" +
	"4746387c211764746387c211764746387c211764746387c211764746387c2117" +
	"64746387c211764746387c211764746387c211764746387c211764746387c211" +
	"764746364000feff7dd9fc1fd398afd3fe407f9719ef4e69370b1f035d6da385" +
	"7f241edceabd55fb65252417525afa030840ffff2039be0787d725e39c44240c" +
	"fef346267f79a3133fbcd1899fde68c4cfef346267f79a3133fbcd1899fde660" +
	"0000"

// webpLosslessHex is a 600×900 lossless WebP (VP8L), 72 bytes: the top half
// green, the bottom half yellow.
//
//	cwebp -lossless big.png -o big.webp
const webpLosslessHex = "5249464640000000574542505650384c340000002f57c2e0000f30fff31ff31f" +
	"1514b46dc3943fe982d87144ff33fce73ffff9cf7ffef39fffc35dfef39ffffc" +
	"e73ffff9cfffe106"

// webpAlphaHex is a 120×180 lossy WebP with an alpha channel (VP8X, ALPH and
// VP8), 222 bytes: the left half transparent, the right half opaque red.
//
//	cwebp -q 80 -alpha_q 100 alpha.png -o alpha.webp
const webpAlphaHex = "52494646d600000057454250565038580a00000010000000770000b30000414c" +
	"504817000000010f30ff11118220dba6feb43798c513d1ff00acaa3a9f005650" +
	"382098000000700a009d012a7800b4003e6d369748a422a221247eb800800d89" +
	"676ee173e5f00abffdc000164026dab02d5f0799fa4abdbb960592c76cf5abe0" +
	"f33f4957b772c0b258ed9eb57c1e67e92af6ee58164b1db3d6af83ccfd255edd" +
	"ca8800feff83749fd68efffcfb5cffddef2ed37b9f603ffff4b0fcbd74569fff" +
	"161f97ae889983dcdb3cd5e941b6c9f80db64fc06db27e036d93f01ae000"

// webpOf is the WebP file that hexData spells, labelled as a WebP.
func webpOf(t *testing.T, hexData string) abs.Image {
	t.Helper()
	data, err := hex.DecodeString(hexData)
	require.NoError(t, err)
	return abs.Image{Data: data, MIMEType: "image/webp"}
}

// webpClaiming is a WebP whose header says it is w×h, 16384 at most each way.
// Its pixels are those of the 600×900 picture of [webpLosslessHex].
func webpClaiming(t *testing.T, w, h int) abs.Image {
	t.Helper()
	img := webpOf(t, webpLosslessHex)
	// "RIFF", the size, "WEBP", the "VP8L" chunk header of 8 bytes, then the
	// signature byte 0x2f and 4 bytes: 14 bits of width less one, 14 of height
	// less one, a bit for alpha and 3 of version.
	require.Equal(t, "VP8L", string(img.Data[12:16]))
	require.Equal(t, byte(0x2f), img.Data[20])
	binary.LittleEndian.PutUint32(img.Data[21:], uint32(w-1)|uint32(h-1)<<14) //nolint:gosec // 14 bits each
	return img
}

// cut is img without its bytes from n on.
func cut(img abs.Image, n int) abs.Image {
	return abs.Image{Data: img.Data[:n], MIMEType: img.MIMEType}
}

// assertColour checks that the pixel of img at (x, y) is want, give or take
// delta on each channel: a lossy file does not keep a colour exactly.
func assertColour(t *testing.T, img image.Image, x, y int, want color.RGBA, delta float64) {
	t.Helper()
	r, g, b := rgbAt(img, x, y)
	assert.InDelta(t, want.R, r, delta, "red at (%d, %d)", x, y)
	assert.InDelta(t, want.G, g, delta, "green at (%d, %d)", x, y)
	assert.InDelta(t, want.B, b, delta, "blue at (%d, %d)", x, y)
}

// pngClaiming is a PNG whose header says it is w×h, with the checksum the
// header needs. Its pixels are those of a 1×1 picture.
func pngClaiming(t *testing.T, w, h uint32) abs.Image {
	t.Helper()
	img := pngOf(t, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	// An 8-byte signature, then the IHDR chunk: 4 bytes of length, "IHDR",
	// width, height, 5 more bytes and 4 of checksum over "IHDR" and the 13.
	require.Equal(t, "IHDR", string(img.Data[12:16]))
	binary.BigEndian.PutUint32(img.Data[16:], w)
	binary.BigEndian.PutUint32(img.Data[20:], h)
	binary.BigEndian.PutUint32(img.Data[29:], crc32.ChecksumIEEE(img.Data[12:29]))
	return img
}

func TestPreview_ScalesLargeJPEG(t *testing.T) {
	in := jpegOf(t, gradient(1200, 1800))

	got, err := preview(in)

	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 400, 600), previewJPEG(t, got).Bounds())
	assert.LessOrEqual(t, len(got.Data), MaxPreviewBytes)
}

func TestPreview_ScalesLargePNGToJPEG(t *testing.T) {
	got, err := preview(pngOf(t, gradient(1000, 1000)))

	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 400, 400), previewJPEG(t, got).Bounds())
}

func TestPreview_ScalesLargeGIFToJPEG(t *testing.T) {
	// A small file, but wide: it is the width that counts.
	in := gifOf(t, gradient(600, 300))
	require.LessOrEqual(t, len(in.Data), MaxPreviewBytes)

	got, err := preview(in)

	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 400, 200), previewJPEG(t, got).Bounds())
}

func TestPreview_KeepsTheAspectRatio(t *testing.T) {
	tests := []struct{ w, h, wantH int }{
		{800, 1200, 600},
		{401, 500, 499}, // 498.75 rounds up
		{420, 501, 477}, // 477.14 rounds down
		{4000, 1, 1},    // at least one pixel
		{2400, 3, 1},    // 0.5 is one
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%dx%d", tt.w, tt.h), func(t *testing.T) {
			got, err := preview(pngOf(t, image.NewNRGBA(image.Rect(0, 0, tt.w, tt.h))))

			require.NoError(t, err)
			assert.Equal(t, image.Rect(0, 0, 400, tt.wantH), previewJPEG(t, got).Bounds())
		})
	}
}

func TestPreview_FitsInsideTheLimitsKeepingTheAspectRatio(t *testing.T) {
	tests := []struct {
		name         string
		w, h         int
		wantW, wantH int
	}{
		{"tall, narrow", 300, 1000, 240, 800},
		{"tall, as wide as a preview", 400, 1600, 200, 800},
		{"tall and wide", 1000, 4000, 200, 800},
		{"one pixel over in height", 400, 801, 400, 800},
		{"a strip", 400, 20000, 16, 800},
		{"a thin strip is one pixel wide", 1, 5000, 1, 800},
		{"a very thin one stays one pixel", 3, 32000, 1, 800},
		{"wide wins when both are over, in proportion", 1600, 3000, 400, 750},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := preview(pngOf(t, image.NewGray(image.Rect(0, 0, tt.w, tt.h))))

			require.NoError(t, err)
			assert.Equal(t, image.Rect(0, 0, tt.wantW, tt.wantH), previewJPEG(t, got).Bounds())
			assert.LessOrEqual(t, tt.wantW, PreviewWidth)
			assert.LessOrEqual(t, tt.wantH, PreviewMaxHeight)
		})
	}
}

func TestPreview_ScalingAveragesPixels(t *testing.T) {
	// Every 2×2 block has two black and two white pixels, so the average of a
	// block is grey. Picking one pixel in each block would give black or white.
	board := image.NewGray(image.Rect(0, 0, 800, 800))
	for y := range 800 {
		for x := range 800 {
			if (x+y)%2 == 0 {
				board.SetGray(x, y, color.Gray{Y: 255})
			}
		}
	}

	got, err := preview(pngOf(t, board))

	require.NoError(t, err)
	decoded := previewJPEG(t, got)
	for _, p := range []image.Point{{0, 0}, {123, 45}, {399, 399}} {
		r, g, b := rgbAt(decoded, p.X, p.Y)
		assert.InDelta(t, 128, int(r), 3, "red at %v", p)
		assert.InDelta(t, 128, int(g), 3, "green at %v", p)
		assert.InDelta(t, 128, int(b), 3, "blue at %v", p)
	}
}

func TestPreview_TransparentPixelsComeOutOverWhite(t *testing.T) {
	// Three bands, each 400 pixels wide: transparent, half black, opaque red.
	src := image.NewNRGBA(image.Rect(0, 0, 1200, 600))
	for y := range 600 {
		for x := range 1200 {
			switch {
			case x >= 800:
				src.SetNRGBA(x, y, color.NRGBA{R: 255, A: 255})
			case x >= 400:
				src.SetNRGBA(x, y, color.NRGBA{A: 128})
			}
		}
	}

	got, err := preview(pngOf(t, src))

	require.NoError(t, err)
	decoded := previewJPEG(t, got)
	r, g, b := rgbAt(decoded, 60, 100)
	assert.Greater(t, min(r, g, b), uint8(245), "transparent is white, not black")
	r, g, b = rgbAt(decoded, 200, 100)
	assert.InDelta(t, 127, int(r), 4, "half black over white is grey")
	assert.InDelta(t, 127, int(g), 4)
	assert.InDelta(t, 127, int(b), 4)
	r, g, b = rgbAt(decoded, 330, 100)
	assert.Greater(t, r, uint8(240), "opaque red stays red")
	assert.Less(t, max(g, b), uint8(15))
}

func TestPreview_KeepsSmallImage(t *testing.T) {
	for _, in := range []abs.Image{
		jpegOf(t, gradient(300, 450)),
		pngOf(t, gradient(400, 600)), // as wide as a preview may be
		gifOf(t, gradient(200, 300)),
		jpegOf(t, gradient(100, 800)), // as tall as a preview may be
	} {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(in.Data))
		require.NoError(t, err)
		t.Run(fmt.Sprintf("%s %dx%d", in.MIMEType, cfg.Width, cfg.Height), func(t *testing.T) {
			require.LessOrEqual(t, len(in.Data), MaxPreviewBytes)

			got, err := preview(in)

			require.NoError(t, err)
			assert.Equal(t, in, got, "same bytes, same type")
		})
	}
}

func TestPreview_ReencodesANarrowImageOverTheLimit(t *testing.T) {
	in := pngOf(t, noise(300, 300))
	require.Greater(t, len(in.Data), MaxPreviewBytes)

	got, err := preview(in)

	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 300, 300), previewJPEG(t, got).Bounds(), "its own size")
}

func TestPreview_ScalesATallImageUnderTheByteLimit(t *testing.T) {
	// Narrow and small in bytes, but taller than a preview: scaled all the same.
	in := pngOf(t, image.NewGray(image.Rect(0, 0, 200, 1600)))
	require.LessOrEqual(t, len(in.Data), MaxPreviewBytes)

	got, err := preview(in)

	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 100, 800), previewJPEG(t, got).Bounds())
	assert.NotEqual(t, in.Data, got.Data)
}

func TestPreview_ScalesAWideImageUnderTheLimit(t *testing.T) {
	// Under the byte limit, but wider than a preview: scaled all the same.
	in := jpegOf(t, gradient(500, 500))
	require.LessOrEqual(t, len(in.Data), MaxPreviewBytes)

	got, err := preview(in)

	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 400, 400), previewJPEG(t, got).Bounds())
}

func TestPreview_ASmallWebPIsReencodedAsJPEG(t *testing.T) {
	// FantLab's covers are lossy WebP. This one fits and is tiny, and is a JPEG
	// all the same: some apps don't take a WebP image block.
	in := webpOf(t, webpLossyHex)
	require.LessOrEqual(t, len(in.Data), MaxPreviewBytes)

	got, err := preview(in)

	require.NoError(t, err)
	decoded := previewJPEG(t, got)
	assert.Equal(t, image.Rect(0, 0, 200, 300), decoded.Bounds(), "its own size")
	assertColour(t, decoded, 50, 150, color.RGBA{B: 255}, 30)
	assertColour(t, decoded, 150, 150, color.RGBA{R: 255}, 30)
}

func TestPreview_ALargeWebPIsScaledToFitAsJPEG(t *testing.T) {
	in := webpOf(t, webpLosslessHex)

	got, err := preview(in)

	require.NoError(t, err)
	decoded := previewJPEG(t, got)
	assert.Equal(t, image.Rect(0, 0, 400, 600), decoded.Bounds(), "600×900 in proportion")
	assertColour(t, decoded, 200, 100, color.RGBA{G: 255}, 30)
	assertColour(t, decoded, 200, 500, color.RGBA{R: 255, G: 255}, 30)
}

func TestPreview_WebPTransparencyComesOutOverWhite(t *testing.T) {
	got, err := preview(webpOf(t, webpAlphaHex))

	require.NoError(t, err)
	decoded := previewJPEG(t, got)
	assert.Equal(t, image.Rect(0, 0, 120, 180), decoded.Bounds())
	r, g, b := rgbAt(decoded, 20, 90)
	assert.Greater(t, min(r, g, b), uint8(245), "transparent is white, not black")
	assertColour(t, decoded, 100, 90, color.RGBA{R: 255}, 30)
}

func TestPreview_GoesByTheBytesNotByTheLabel(t *testing.T) {
	t.Run("a WebP labelled as a JPEG is a WebP", func(t *testing.T) {
		in := webpOf(t, webpLossyHex)
		in.MIMEType = "image/jpeg" // as FantLab's Content-Type says

		got, err := preview(in)

		require.NoError(t, err)
		assert.Equal(t, image.Rect(0, 0, 200, 300), previewJPEG(t, got).Bounds())
		assert.NotEqual(t, in.Data, got.Data)
	})
	t.Run("a PNG labelled as a WebP is a PNG", func(t *testing.T) {
		in := pngOf(t, gradient(200, 300))
		in.MIMEType = "image/webp"

		got, err := preview(in)

		require.NoError(t, err)
		assert.Equal(t, in, got, "kept as it is, label and all")
	})
}

func TestPreview_ACutWebPIsNotAnImage(t *testing.T) {
	lossy, lossless, alpha := webpOf(t, webpLossyHex), webpOf(t, webpLosslessHex), webpOf(t, webpAlphaHex)
	tests := []struct {
		name string
		in   abs.Image
	}{
		{"lossy cut in its header", cut(lossy, 24)},
		{"lossy cut in its pixels", cut(lossy, len(lossy.Data)/2)},
		{"lossy without its last byte", cut(lossy, len(lossy.Data)-1)},
		{"lossless cut in its header", cut(lossless, 24)},
		{"lossless cut in its pixels", cut(lossless, len(lossless.Data)/2)},
		{"alpha cut in its alpha channel", cut(alpha, 40)},
		{"alpha cut in its pixels", cut(alpha, len(alpha.Data)-60)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := preview(tt.in)

			require.ErrorIs(t, err, errNotImage)
			assert.Equal(t, "not an image", problemOf(err))
			assert.Equal(t, abs.Image{}, got)
		})
	}
}

func TestPreview_RefusesAWebPWithTooManyPixels(t *testing.T) {
	for _, tt := range []struct{ w, h int }{{4001, 4000}, {16000, 16000}} {
		t.Run(fmt.Sprintf("%dx%d", tt.w, tt.h), func(t *testing.T) {
			in := webpClaiming(t, tt.w, tt.h)
			cfg, format, err := image.DecodeConfig(bytes.NewReader(in.Data))
			require.NoError(t, err)
			require.Equal(t, "webp", format)
			require.Equal(t, image.Config{ColorModel: cfg.ColorModel, Width: tt.w, Height: tt.h}, cfg)

			got, err := preview(in)

			// The pixels are those of a 600×900 picture, so decoding them would
			// fail in another way: this is the refusal, before any decoding.
			require.ErrorIs(t, err, errPreviewTooLarge)
			assert.Equal(t, "too large to preview", problemOf(err))
			assert.Equal(t, abs.Image{}, got)
		})
	}
}

func TestPreview_AFormatThatCannotBeDecodedCannotBePreviewed(t *testing.T) {
	// BMP, of any size: nothing registers a decoder for it, so it can't be
	// looked at, and it is not passed on unchecked.
	for _, size := range []int{20, MaxPreviewBytes - 1, MaxPreviewBytes, MaxPreviewBytes + 1} {
		got, err := preview(bmp(size))

		require.ErrorIs(t, err, errCannotPreview, "%d bytes", size)
		assert.Equal(t, "can't be previewed", problemOf(err), "%d bytes", size)
		assert.Equal(t, abs.Image{}, got, "%d bytes", size)
	}
}

func TestPreview_RefusesHugeDimensions(t *testing.T) {
	tests := []struct {
		name string
		w, h uint32
	}{
		{"huge", 20000, 20000},
		{"wide", 100000, 200},
		{"one pixel over", 4001, 4000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := pngClaiming(t, tt.w, tt.h)
			cfg, _, err := image.DecodeConfig(bytes.NewReader(in.Data))
			require.NoError(t, err)
			require.Equal(t, image.Config{ColorModel: cfg.ColorModel, Width: int(tt.w), Height: int(tt.h)}, cfg)

			got, err := preview(in)

			// The pixels are those of a 1×1 picture, so decoding them would fail
			// in another way: this is the refusal, before any decoding.
			require.ErrorIs(t, err, errPreviewTooLarge)
			assert.Equal(t, "too large to preview", problemOf(err))
			assert.Equal(t, abs.Image{}, got)
		})
	}
}

func TestPreview_GarbageClaimingToBeAnImage(t *testing.T) {
	big := jpegOf(t, gradient(500, 500))
	wide := pngOf(t, gradient(500, 500))
	// Small ones, whose size and header are within every limit: only decoding
	// shows they are cut.
	small := jpegOf(t, gradient(100, 150))
	smallPNG := pngOf(t, gradient(100, 150))
	smallGIF := gifOf(t, gradient(100, 150))
	tests := []struct {
		name string
		in   abs.Image
	}{
		{"jpeg cut in its header", abs.Image{Data: big.Data[:10], MIMEType: "image/jpeg"}},
		{"jpeg cut in its pixels", abs.Image{Data: big.Data[:len(big.Data)/2], MIMEType: "image/jpeg"}},
		{"png cut in its pixels", abs.Image{Data: wide.Data[:len(wide.Data)/2], MIMEType: "image/png"}},
		{"png cut in its header", abs.Image{Data: wide.Data[:12], MIMEType: "image/png"}},
		{"gif cut in its header", abs.Image{Data: []byte("GIF89a\x01"), MIMEType: "image/gif"}},
		{"small jpeg cut in its pixels", abs.Image{Data: small.Data[:len(small.Data)/2], MIMEType: "image/jpeg"}},
		{"small png cut in its pixels", abs.Image{Data: smallPNG.Data[:len(smallPNG.Data)/2], MIMEType: "image/png"}},
		{"small gif cut in its pixels", abs.Image{Data: smallGIF.Data[:len(smallGIF.Data)/2], MIMEType: "image/gif"}},
		{"small jpeg without its last byte", abs.Image{Data: small.Data[:len(small.Data)-1], MIMEType: "image/jpeg"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := preview(tt.in)

			require.ErrorIs(t, err, errNotImage)
			assert.Equal(t, "not an image", problemOf(err))
			assert.Equal(t, abs.Image{}, got)
		})
	}
}

func TestPreview_DoesNotChangeItsInput(t *testing.T) {
	in := jpegOf(t, gradient(500, 500))
	before := bytes.Clone(in.Data)

	_, err := preview(in)

	require.NoError(t, err)
	assert.Equal(t, before, in.Data)
}

func TestFind_PreviewsAreSmall(t *testing.T) {
	big := jpegOf(t, gradient(1200, 1800))
	urls := coverURLs(MaxCoverCandidates)
	fetch := NewMockImageFetcher(t)
	fetch.EXPECT().Fetch(mock.Anything, mock.Anything).Return(big, nil)

	got, err := NewCovers(coversAPI(t, "google", urls), fetch, "https://abs.example").Find(t.Context(), "li_1", "google", "", "")

	require.NoError(t, err)
	require.Len(t, got.Candidates, MaxCoverCandidates)
	total := 0
	for i, c := range got.Candidates {
		assert.Equal(t, urls[i], c.URL)
		assert.Empty(t, c.Problem)
		require.NotNil(t, c.Image)
		cfg, _, err := image.DecodeConfig(bytes.NewReader(c.Image.Data))
		require.NoError(t, err)
		assert.LessOrEqual(t, cfg.Width, PreviewWidth)
		total += len(c.Image.Data)
	}
	assert.Less(t, total, 1<<20, "the five together")
	assert.Equal(t, &solarisCover, got.Current, "the current cover is already small, and is not touched")
}

func TestFind_AnImageThatCannotBeShownIsAProblemOfItsOwn(t *testing.T) {
	urls := coverURLs(4)
	fetch := NewMockImageFetcher(t)
	fetch.EXPECT().Fetch(mock.Anything, urls[0]).Return(bmp(1000), nil)
	fetch.EXPECT().Fetch(mock.Anything, urls[1]).Return(abs.Image{Data: []byte("\xff\xd8\xff\xe0\x00\x10JFIF"), MIMEType: "image/jpeg"}, nil)
	fetch.EXPECT().Fetch(mock.Anything, urls[2]).Return(pngClaiming(t, 20000, 20000), nil)
	fetch.EXPECT().Fetch(mock.Anything, urls[3]).RunAndReturn(func(context.Context, string) (abs.Image, error) {
		return jpegOf(t, gradient(500, 750)), nil
	})

	got, err := NewCovers(coversAPI(t, "google", urls), fetch, "https://abs.example").Find(t.Context(), "li_1", "google", "", "")

	require.NoError(t, err)
	require.Len(t, got.Candidates, 4)
	assert.Equal(t, CoverCandidate{URL: urls[0], Problem: "can't be previewed"}, got.Candidates[0])
	assert.Equal(t, CoverCandidate{URL: urls[1], Problem: "not an image"}, got.Candidates[1])
	assert.Equal(t, CoverCandidate{URL: urls[2], Problem: "too large to preview"}, got.Candidates[2])
	assert.Empty(t, got.Candidates[3].Problem)
	require.NotNil(t, got.Candidates[3].Image)
	assert.Equal(t, image.Rect(0, 0, 400, 600), previewJPEG(t, *got.Candidates[3].Image).Bounds())
}

func TestFind_ATallStripIsScaledToFit(t *testing.T) {
	// 500×32000 is the most pixels a preview may be made from, and
	// the most it can be out of proportion: a strip, not a cover.
	strip := pngOf(t, image.NewGray(image.Rect(0, 0, 500, 32000)))
	urls := coverURLs(1)
	fetch := NewMockImageFetcher(t)
	fetch.EXPECT().Fetch(mock.Anything, urls[0]).Return(strip, nil)

	got, err := NewCovers(coversAPI(t, "google", urls), fetch, "https://abs.example").Find(t.Context(), "li_1", "google", "", "")

	require.NoError(t, err)
	require.Len(t, got.Candidates, 1)
	require.NotNil(t, got.Candidates[0].Image)
	assert.Empty(t, got.Candidates[0].Problem)
	assert.Equal(t, image.Rect(0, 0, 13, PreviewMaxHeight), previewJPEG(t, *got.Candidates[0].Image).Bounds())
}

func TestFind_ATallStripPastThePixelLimitIsTooLarge(t *testing.T) {
	urls := coverURLs(1)
	fetch := NewMockImageFetcher(t)
	fetch.EXPECT().Fetch(mock.Anything, urls[0]).Return(pngClaiming(t, 500, 32001), nil)

	got, err := NewCovers(coversAPI(t, "google", urls), fetch, "https://abs.example").Find(t.Context(), "li_1", "google", "", "")

	require.NoError(t, err)
	assert.Equal(t, []CoverCandidate{{URL: urls[0], Problem: "too large to preview"}}, got.Candidates)
}

func TestFind_AnImageWithinTheLimitsIsPassedOnAsItIs(t *testing.T) {
	in := jpegOf(t, gradient(400, 800))
	require.LessOrEqual(t, len(in.Data), MaxPreviewBytes)
	urls := coverURLs(1)
	fetch := NewMockImageFetcher(t)
	fetch.EXPECT().Fetch(mock.Anything, urls[0]).Return(in, nil)

	got, err := NewCovers(coversAPI(t, "google", urls), fetch, "https://abs.example").Find(t.Context(), "li_1", "google", "", "")

	require.NoError(t, err)
	assert.Equal(t, []CoverCandidate{{URL: urls[0], Image: &in}}, got.Candidates)
}

func TestFind_AWebPLabelledAsJPEGGetsAJPEGPreview(t *testing.T) {
	// FantLab serves WebP bytes as image/jpeg. The fetcher, which goes by the
	// bytes, lets it through, and so does the preview.
	webpData := webpOf(t, webpLossyHex).Data
	srv := serve(t, respond("image/jpeg", webpData))
	urls := []string{srv.URL + "/cover.jpg"}

	got, err := NewCovers(coversAPI(t, "fantlab", urls), NewImageFetcher(), "https://abs.example").Find(t.Context(), "li_1", "fantlab", "", "")

	require.NoError(t, err)
	require.Len(t, got.Candidates, 1)
	assert.Equal(t, urls[0], got.Candidates[0].URL)
	assert.Empty(t, got.Candidates[0].Problem)
	require.NotNil(t, got.Candidates[0].Image)
	assert.Equal(t, image.Rect(0, 0, 200, 300), previewJPEG(t, *got.Candidates[0].Image).Bounds())
}

// observePreviews makes the service preview images through a fake that records
// how many run at once, and returns the most it saw. Every preview takes a
// while, long enough for others to start if they may.
func observePreviews(svc *Covers) *atomic.Int32 { return observePreviewsOf(&svc.preview) }

// observePreviewsOf is [observePreviews] for any service: it replaces the
// preview function at preview.
func observePreviewsOf(preview *func(abs.Image) (abs.Image, error)) *atomic.Int32 {
	var running, most atomic.Int32
	*preview = func(img abs.Image) (abs.Image, error) {
		n := running.Add(1)
		for {
			if m := most.Load(); n <= m || most.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		running.Add(-1)
		return img, nil
	}
	return &most
}

func TestFind_DecodesAtMostTwoPreviewsAtATime(t *testing.T) {
	urls := coverURLs(MaxCoverCandidates)
	svc := NewCovers(coversAPI(t, "google", urls), fetchesEvery(t, urls, nil), "https://abs.example")
	most := observePreviews(svc)

	got, err := svc.Find(t.Context(), "li_1", "google", "", "")

	require.NoError(t, err)
	require.Len(t, got.Candidates, MaxCoverCandidates)
	for i, c := range got.Candidates {
		assert.Equal(t, urls[i], c.URL, "the order of the search is kept")
		assert.NotNil(t, c.Image)
		assert.Empty(t, c.Problem)
	}
	assert.Equal(t, int32(2), most.Load(), "two at once, and no more")
}

func TestFind_OnePreviewLimitForAllSearches(t *testing.T) {
	const searches = 4
	urls := coverURLs(MaxCoverCandidates)
	svc := NewCovers(coversAPI(t, "google", urls), fetchesEvery(t, urls, nil), "https://abs.example")
	most := observePreviews(svc)

	start := make(chan struct{})
	var wg sync.WaitGroup
	got := make([]CoverSearch, searches)
	errs := make([]error, searches)
	for i := range searches {
		wg.Go(func() {
			<-start
			got[i], errs[i] = svc.Find(t.Context(), "li_1", "google", "", "")
		})
	}
	close(start)
	wg.Wait()

	for i := range searches {
		require.NoError(t, errs[i])
		require.Len(t, got[i].Candidates, MaxCoverCandidates)
		for j, c := range got[i].Candidates {
			assert.Equal(t, urls[j], c.URL, "search %d keeps the order of the provider", i)
			assert.NotNil(t, c.Image)
			assert.Empty(t, c.Problem)
		}
	}
	assert.Equal(t, int32(maxConcurrentPreviews), most.Load(), "the limit is for the whole server, not for each search")
}

// coversWithSlots is a Covers service that has no API, makes its previews with
// f and holds them to the given slots.
func coversWithSlots(slots chan struct{}, f func(abs.Image) (abs.Image, error)) *Covers {
	svc := NewCovers(nil, nil, "https://abs.example")
	svc.slots, svc.preview = slots, f
	return svc
}

func TestPreviewWhenFree_GivesUpWhenTheContextEnds(t *testing.T) {
	slots := make(chan struct{}, 1)
	slots <- struct{}{} // the only slot is taken
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	svc := coversWithSlots(slots, func(abs.Image) (abs.Image, error) {
		t.Error("a preview was made without a free slot")
		return abs.Image{}, nil
	})

	_, err := svc.previewWhenFree(ctx, imageOf("x"))

	require.ErrorIs(t, err, context.Canceled)
	assert.Len(t, slots, 1, "the slot of another is not touched")
}

func TestPreviewWhenFree_ReleasesItsSlot(t *testing.T) {
	slots := make(chan struct{}, 1)
	svc := coversWithSlots(slots, func(img abs.Image) (abs.Image, error) { return img, nil })

	for range 3 {
		_, err := svc.previewWhenFree(t.Context(), imageOf("x"))
		require.NoError(t, err)
	}

	assert.Empty(t, slots)
}
