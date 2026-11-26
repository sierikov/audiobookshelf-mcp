package service

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	_ "image/gif" // registers the GIF decoder for image.Decode
	"image/jpeg"
	_ "image/png" // registers the PNG decoder for image.Decode

	_ "golang.org/x/image/webp" // registers the WebP decoder for image.Decode

	"github.com/sierikov/audiobookshelf-mcp/internal/abs"
)

// previewQuality is the JPEG quality of a preview: enough to tell covers apart.
const previewQuality = 80

// formatWebP is the name image.DecodeConfig gives a WebP image.
const formatWebP = "webp"

var (
	// errPreviewTooLarge is an image with too many pixels to decode.
	errPreviewTooLarge = errors.New("too large to preview")
	// errCannotPreview is an image in a format that no decoder is registered
	// for: it can't be looked at, so it is not passed on unchecked.
	errCannotPreview = errors.New("can't be previewed")
)

// preview makes a candidate's image small enough to show, after checking that
// it is one. It reads the size from the header first, and decodes nothing it
// refuses; then it decodes every image it keeps, whole. The format is what the
// bytes are, whatever the image's MIMEType says.
//
// A JPEG, PNG or GIF that decodes, fits inside [PreviewWidth] by
// [PreviewMaxHeight] and is at most [MaxPreviewBytes] comes back as it is. Any
// other is scaled down to fit, keeping its proportions (see [previewSize]), and
// returned as a JPEG; one that fits is a JPEG at its own size. A WebP is always
// such a JPEG, small or not, because some apps don't take a WebP image block.
// Transparent pixels turn white.
//
// An image of more than [MaxPreviewPixels] is an [errPreviewTooLarge] error,
// and one in a format that no decoder reads is an [errCannotPreview] error. An
// image that claims a format it is not in, or cannot be decoded or encoded, is
// an [errNotImage] error.
func preview(img abs.Image) (abs.Image, error) {
	cfg, format, err := image.DecodeConfig(bytes.NewReader(img.Data))
	switch {
	case errors.Is(err, image.ErrFormat):
		return abs.Image{}, errCannotPreview
	case err != nil:
		return abs.Image{}, errNotImage
	case int64(cfg.Width)*int64(cfg.Height) > MaxPreviewPixels:
		return abs.Image{}, errPreviewTooLarge
	}
	src, _, err := image.Decode(bytes.NewReader(img.Data))
	if err != nil || src.Bounds().Empty() {
		return abs.Image{}, errNotImage // whatever the decoder said: its text is not ours to pass on
	}
	b := src.Bounds()
	w, h := previewSize(b)
	if keptAsIs(format, b, w, h, len(img.Data)) {
		return img, nil
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, boxScale(src, w, h), &jpeg.Options{Quality: previewQuality}); err != nil {
		return abs.Image{}, errNotImage
	}
	return abs.Image{Data: buf.Bytes(), MIMEType: "image/jpeg"}, nil
}

// keptAsIs reports whether an image of the given format, bounds b and size in
// bytes is shown as it is: it already fits, which is when w×h, its [previewSize],
// is b, and is at most [MaxPreviewBytes]. A WebP never is.
func keptAsIs(format string, b image.Rectangle, w, h, size int) bool {
	return format != formatWebP && w == b.Dx() && h == b.Dy() && size <= MaxPreviewBytes
}

// previewSize is the size of the preview of a picture of the size of b: its own
// size if it fits inside [PreviewWidth] by [PreviewMaxHeight]. Otherwise it is
// scaled down by the larger of the two ratios, keeping the proportions, each
// side rounded and at least 1.
func previewSize(b image.Rectangle) (w, h int) {
	w, h = b.Dx(), b.Dy()
	switch {
	case w <= PreviewWidth && h <= PreviewMaxHeight:
		return w, h
	case w*PreviewMaxHeight >= h*PreviewWidth: // w/PreviewWidth >= h/PreviewMaxHeight: the width limit counts
		return PreviewWidth, max(1, (h*PreviewWidth+w/2)/w)
	}
	return max(1, (w*PreviewMaxHeight+h/2)/h), PreviewMaxHeight
}

// boxScale scales src down to w×h, no larger than src. Each pixel of the result
// is the average of the pixels of src it stands for, a simple filter and good
// enough to compare covers. Transparent pixels turn white.
func boxScale(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		y0, y1 := b.Min.Y+y*b.Dy()/h, b.Min.Y+(y+1)*b.Dy()/h
		for x := range w {
			x0, x1 := b.Min.X+x*b.Dx()/w, b.Min.X+(x+1)*b.Dx()/w
			dst.SetRGBA(x, y, boxAverage(src, image.Rect(x0, y0, max(x1, x0+1), max(y1, y0+1))))
		}
	}
	return dst
}

// boxAverage is the average colour of the pixels of src in r, which is not
// empty, over a white background.
func boxAverage(src image.Image, r image.Rectangle) color.RGBA {
	var sumR, sumG, sumB, sumA uint64
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			pr, pg, pb, pa := src.At(x, y).RGBA()
			sumR, sumG, sumB, sumA = sumR+uint64(pr), sumG+uint64(pg), sumB+uint64(pb), sumA+uint64(pa)
		}
	}
	n := uint64(r.Dx() * r.Dy()) //nolint:gosec // r is not empty
	// The colours are premultiplied by alpha, so a white background shows
	// through by what alpha leaves open: 0xffff minus it.
	white := 0xffff - sumA/n
	return color.RGBA{R: to8(sumR/n + white), G: to8(sumG/n + white), B: to8(sumB/n + white), A: 0xff}
}

// to8 is the 8 bits of a 16-bit channel value that matter.
func to8(v uint64) uint8 {
	return uint8(min(v, 0xffff) >> 8) //nolint:gosec // at most 0xff
}
