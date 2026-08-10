package optimizer

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"

	"github.com/disintegration/imaging"
	"github.com/ericpauley/go-quantize/quantize"
)

// Uploaded images are optimized for e-mail delivery: downsized to
// MaxImageWidth when wider (never upsized) and recompressed. Whatever the
// encoders produce, the original bytes always win unless the optimized
// output is strictly smaller — the pipeline can therefore never make a
// file worse, only refuse to improve it.
const (
	MaxImageWidth = 1200
	JPEGQuality   = 80

	// MaxImagePixels caps the decoded size of an upload. Pixels decode to
	// ~4 bytes each, so 50 MP ≈ 200 MB — survivable once, but anything
	// beyond it risks OOMing a small host. Checked against the header
	// before any full decode, so a decompression-bomb image is rejected
	// while still cheap.
	MaxImagePixels = 50_000_000
)

// Image is the outcome of Optimize. Ext and ContentType may
// differ from the upload's (an opaque photographic PNG becomes a JPEG).
type Image struct {
	Data        []byte
	Ext         string
	ContentType string
	Width       int
	Height      int
}

// Optimize downsizes and recompresses a raster upload for e-mail
// delivery. Animated GIFs are only touched when wider than MaxImageWidth:
// re-encoding an already frame-diffed GIF routinely inflates it, so
// in-bounds animations pass through byte-identical.
func Optimize(raw []byte, ext string) (Image, error) {
	// Reject decompression bombs from the header alone, before anything
	// allocates pixel buffers. An unparseable header falls through to the
	// full decode below, which produces the proper error.
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(raw)); err == nil && cfg.Width*cfg.Height > MaxImagePixels {
		return Image{}, fmt.Errorf("image dimensions %dx%d exceed the %d megapixel limit",
			cfg.Width, cfg.Height, MaxImagePixels/1_000_000)
	}

	if ext == "gif" {
		if g, err := gif.DecodeAll(bytes.NewReader(raw)); err == nil && len(g.Image) > 1 {
			return optimizeAnimatedGIF(raw, g), nil
		}
		// A single-frame GIF joins the still-image path below.
	}

	img, err := imaging.Decode(bytes.NewReader(raw), imaging.AutoOrientation(true))
	if err != nil {
		return Image{}, err
	}

	// The baseline carries the original bytes, so it must carry the
	// original dimensions too — the resized dims apply only to candidates
	// actually encoded from the resized pixels.
	best := Image{
		Data:        raw,
		Ext:         ext,
		ContentType: rasterContentType(ext),
		Width:       img.Bounds().Dx(),
		Height:      img.Bounds().Dy(),
	}

	if img.Bounds().Dx() > MaxImageWidth {
		img = imaging.Resize(img, MaxImageWidth, 0, imaging.Lanczos)
	}

	// Palette-quantized PNG: the strongest email-safe encoding for flat
	// graphics, and the only lossy candidate that preserves transparency.
	if p, err := encodeQuantizedPNG(img); err == nil && len(p) < len(best.Data) {
		best = Image{Data: p, Ext: "png", ContentType: "image/png",
			Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}
	}

	// JPEG: usually the far smaller candidate for photographic content,
	// but it has no alpha channel, so opaque images only.
	if isOpaque(img) {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: JPEGQuality}); err == nil && out.Len() < len(best.Data) {
			best = Image{Data: out.Bytes(), Ext: "jpg", ContentType: "image/jpeg",
				Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}
		}
	}

	return best, nil
}

// optimizeAnimatedGIF resizes an over-wide animation frame by frame:
// each frame is coalesced onto a full canvas, resized, and re-dithered
// against its own original palette. If the re-encode is not strictly
// smaller than the upload, the upload wins.
func optimizeAnimatedGIF(raw []byte, g *gif.GIF) Image {
	orig := Image{
		Data:        raw,
		Ext:         "gif",
		ContentType: "image/gif",
		Width:       g.Config.Width,
		Height:      g.Config.Height,
	}
	if g.Config.Width <= MaxImageWidth {
		return orig
	}

	var (
		canvas = image.NewRGBA(image.Rect(0, 0, g.Config.Width, g.Config.Height))
		out    = &gif.GIF{LoopCount: g.LoopCount}
		newW   int
		newH   int
	)
	for i, frame := range g.Image {
		// Honor the source frame's disposal method: DisposalPrevious needs
		// the pre-frame canvas restored afterwards, DisposalBackground
		// clears the frame's rect. Skipping this leaves ghost trails in
		// animations that rely on a clear step between frames.
		disposal := byte(gif.DisposalNone)
		if i < len(g.Disposal) {
			disposal = g.Disposal[i]
		}
		var snapshot *image.RGBA
		if disposal == gif.DisposalPrevious {
			snapshot = image.NewRGBA(canvas.Rect)
			copy(snapshot.Pix, canvas.Pix)
		}

		draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
		resized := imaging.Resize(canvas, MaxImageWidth, 0, imaging.Lanczos)
		newW, newH = resized.Bounds().Dx(), resized.Bounds().Dy()

		pal := image.NewPaletted(resized.Bounds(), frame.Palette)
		draw.FloydSteinberg.Draw(pal, resized.Bounds(), resized, image.Point{})
		out.Image = append(out.Image, pal)
		out.Delay = append(out.Delay, g.Delay[i])
		// Output frames are coalesced to the full canvas, so they carry no
		// disposal of their own.
		out.Disposal = append(out.Disposal, gif.DisposalNone)

		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frame.Bounds(), image.Transparent, image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			canvas = snapshot
		}
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, out); err != nil || buf.Len() >= len(raw) {
		return orig
	}
	return Image{Data: buf.Bytes(), Ext: "gif", ContentType: "image/gif", Width: newW, Height: newH}
}

// encodeQuantizedPNG palette-quantizes an image to at most 256 colors
// (transparency included) and encodes it as PNG at best compression.
func encodeQuantizedPNG(img image.Image) ([]byte, error) {
	var (
		q   = quantize.MedianCutQuantizer{}
		pal = q.Quantize(make([]color.Color, 0, 256), img)
		dst = image.NewPaletted(img.Bounds(), pal)
	)
	draw.FloydSteinberg.Draw(dst, img.Bounds(), img, image.Point{})

	var out bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// isOpaque reports whether every pixel is fully opaque.
func isOpaque(img image.Image) bool {
	if o, ok := img.(interface{ Opaque() bool }); ok {
		return o.Opaque()
	}
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a != 0xffff {
				return false
			}
		}
	}
	return true
}

// rasterContentType maps a raster extension to its MIME type.
func rasterContentType(ext string) string {
	switch ext {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	}
	return "application/octet-stream"
}
