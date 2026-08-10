package optimizer

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand"
	"strings"
	"testing"
)

// noiseImage builds a photographic-looking opaque image: per-pixel noise
// defeats PNG compression the way real photos do.
func noiseImage(w, h int) *image.RGBA {
	rng := rand.New(rand.NewSource(42))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = uint8(rng.Intn(256))
	}
	// Force full opacity.
	for i := 3; i < len(img.Pix); i += 4 {
		img.Pix[i] = 0xff
	}
	return img
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestOptimizePhotoPNGBecomesJPEG(t *testing.T) {
	raw := encodePNG(t, noiseImage(400, 300))
	opt, err := Optimize(raw, "png")
	if err != nil {
		t.Fatal(err)
	}
	if opt.Ext != "jpg" || opt.ContentType != "image/jpeg" {
		t.Fatalf("photographic PNG should convert to JPEG, got ext=%q type=%q", opt.Ext, opt.ContentType)
	}
	if len(opt.Data) >= len(raw) {
		t.Fatalf("optimized output (%d) not smaller than original (%d)", len(opt.Data), len(raw))
	}
	if opt.Width != 400 || opt.Height != 300 {
		t.Fatalf("dimensions changed unexpectedly: %dx%d", opt.Width, opt.Height)
	}
	if _, err := jpeg.Decode(bytes.NewReader(opt.Data)); err != nil {
		t.Fatalf("output is not decodable JPEG: %v", err)
	}
}

func TestOptimizeDownsizesWideImage(t *testing.T) {
	raw := encodePNG(t, noiseImage(2400, 600))
	opt, err := Optimize(raw, "png")
	if err != nil {
		t.Fatal(err)
	}
	if opt.Width != MaxImageWidth {
		t.Fatalf("width = %d, want %d", opt.Width, MaxImageWidth)
	}
	if opt.Height != 300 {
		t.Fatalf("height = %d, want aspect-preserving 300", opt.Height)
	}
	if len(opt.Data) >= len(raw) {
		t.Fatalf("downsized output (%d) not smaller than original (%d)", len(opt.Data), len(raw))
	}
}

func TestOptimizeNeverUpsizes(t *testing.T) {
	raw := encodePNG(t, noiseImage(50, 40))
	opt, err := Optimize(raw, "png")
	if err != nil {
		t.Fatal(err)
	}
	if opt.Width != 50 || opt.Height != 40 {
		t.Fatalf("small image was resized to %dx%d", opt.Width, opt.Height)
	}
}

func TestOptimizePreservesAlpha(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 120, 120))
	// Opaque red square on a fully transparent field.
	draw.Draw(img, image.Rect(30, 30, 90, 90), image.NewUniform(color.NRGBA{R: 200, A: 255}), image.Point{}, draw.Src)
	raw := encodePNG(t, img)

	opt, err := Optimize(raw, "png")
	if err != nil {
		t.Fatal(err)
	}
	if opt.Ext != "png" {
		t.Fatalf("transparent image must stay PNG, got %q", opt.Ext)
	}
	out, _, err := image.Decode(bytes.NewReader(opt.Data))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := out.At(0, 0).RGBA(); a != 0 {
		t.Fatalf("transparent corner became opaque (alpha=%d)", a)
	}
	if _, _, _, a := out.At(60, 60).RGBA(); a != 0xffff {
		t.Fatalf("opaque center lost opacity (alpha=%d)", a)
	}
}

func TestOptimizeKeepsOriginalWhenNotSmaller(t *testing.T) {
	// A tiny, already-optimal JPEG: re-encoding cannot beat it.
	var out bytes.Buffer
	if err := jpeg.Encode(&out, image.NewGray(image.Rect(0, 0, 8, 8)), &jpeg.Options{Quality: 10}); err != nil {
		t.Fatal(err)
	}
	raw := out.Bytes()
	opt, err := Optimize(raw, "jpg")
	if err != nil {
		t.Fatal(err)
	}
	if len(opt.Data) > len(raw) {
		t.Fatalf("output (%d bytes) larger than original (%d)", len(opt.Data), len(raw))
	}
}

func makeAnimatedGIF(t *testing.T, w, h, frames int) []byte {
	t.Helper()
	g := &gif.GIF{}
	pal := color.Palette{color.Black, color.White, color.NRGBA{R: 255, A: 255}, color.NRGBA{B: 255, A: 255}}
	for i := 0; i < frames; i++ {
		f := image.NewPaletted(image.Rect(0, 0, w, h), pal)
		for x := 0; x < w; x++ {
			for y := 0; y < h; y++ {
				f.SetColorIndex(x, y, uint8((x/10+y/10+i)%4))
			}
		}
		g.Image = append(g.Image, f)
		g.Delay = append(g.Delay, 10)
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	}
	var out bytes.Buffer
	if err := gif.EncodeAll(&out, g); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestRejectsDecompressionBomb(t *testing.T) {
	// A syntactically valid PNG signature + IHDR claiming 100000x100000
	// (10 gigapixels ≈ 40 GB decoded). The guard reads only the header,
	// so no pixel data is needed — and none must ever be decoded.
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:], 100000) // width
	binary.BigEndian.PutUint32(ihdr[4:], 100000) // height
	ihdr[8] = 8                                  // bit depth
	ihdr[9] = 2                                  // color type: truecolor

	var raw bytes.Buffer
	raw.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	var chunk bytes.Buffer
	binary.Write(&chunk, binary.BigEndian, uint32(len(ihdr)))
	chunk.WriteString("IHDR")
	chunk.Write(ihdr)
	crc := crc32.NewIEEE()
	crc.Write([]byte("IHDR"))
	crc.Write(ihdr)
	binary.Write(&chunk, binary.BigEndian, crc.Sum32())
	raw.Write(chunk.Bytes())

	if _, err := Optimize(raw.Bytes(), "png"); err == nil {
		t.Fatal("10-gigapixel image was not rejected")
	} else if !strings.Contains(err.Error(), "megapixel limit") {
		t.Fatalf("rejected, but not by the pixel guard: %v", err)
	}
}

func TestKeptOriginalReportsOriginalDimensions(t *testing.T) {
	// A 1201px-wide 4-color paletted noise PNG, barely over the cap: the
	// original's index stream is ~2 bits/px, but resizing 1201→1200 with
	// Lanczos interpolates the 4 colors into many, so the re-quantized,
	// dithered candidate balloons to ~8 bits/px and loses to the guard.
	// Transparency in the palette keeps the JPEG candidate out entirely.
	// The reported dimensions must then be the original's, not the
	// resized candidate's.
	const w, h = 1201, 100
	rng := rand.New(rand.NewSource(9))
	pal := color.Palette{
		color.NRGBA{R: 255, A: 255},
		color.NRGBA{G: 255, A: 128},
		color.NRGBA{B: 255, A: 64},
		color.NRGBA{},
	}
	img := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	for i := range img.Pix {
		img.Pix[i] = uint8(rng.Intn(4))
	}
	raw := encodePNG(t, img)

	opt, err := Optimize(raw, "png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(opt.Data, raw) {
		t.Fatal("fixture no longer defeats the encoders; rebuild it so the keep-original branch is exercised")
	}
	if opt.Width != w || opt.Height != h {
		t.Fatalf("kept original bytes but reported %dx%d, want %dx%d", opt.Width, opt.Height, w, h)
	}
}

func TestAnimatedGIFHonorsBackgroundDisposal(t *testing.T) {
	// Frame 0 covers the full 2400px canvas: solid red on the left half,
	// noise on the right (noise keeps the original large enough that the
	// resized re-encode wins over the keep-original guard). Its disposal
	// is DisposalBackground, so before frame 1 — which only covers the
	// right half — the whole canvas must be cleared. In the coalesced
	// output, frame 1's left half must therefore be transparent; with
	// disposal ignored it would still be frame 0's red.
	const w, h = 2400, 100
	rng := rand.New(rand.NewSource(7))
	pal := color.Palette{color.Transparent, color.NRGBA{R: 255, A: 255}}
	for i := 0; i < 16; i++ {
		v := uint8(i * 16)
		pal = append(pal, color.NRGBA{R: v, G: v, B: v, A: 255})
	}

	f0 := image.NewPaletted(image.Rect(0, 0, w, h), pal)
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			if x < w/2 {
				f0.SetColorIndex(x, y, 1) // red
			} else {
				f0.SetColorIndex(x, y, uint8(2+rng.Intn(16)))
			}
		}
	}
	f1 := image.NewPaletted(image.Rect(w/2, 0, w, h), pal)
	for x := w / 2; x < w; x++ {
		for y := 0; y < h; y++ {
			f1.SetColorIndex(x, y, uint8(2+rng.Intn(16)))
		}
	}

	g := &gif.GIF{
		Image:    []*image.Paletted{f0, f1},
		Delay:    []int{10, 10},
		Disposal: []byte{gif.DisposalBackground, gif.DisposalNone},
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		t.Fatal(err)
	}

	opt, err := Optimize(buf.Bytes(), "gif")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(opt.Data, buf.Bytes()) {
		t.Fatal("keep-original guard fired; rebuild the fixture so the re-encode wins and the disposal path is exercised")
	}
	out, err := gif.DecodeAll(bytes.NewReader(opt.Data))
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Image) != 2 {
		t.Fatalf("frame count = %d, want 2", len(out.Image))
	}
	// Sample well inside the left half of coalesced frame 1.
	if _, _, _, a := out.Image[1].At(out.Image[1].Bounds().Dx()/4, out.Image[1].Bounds().Dy()/2).RGBA(); a != 0 {
		t.Fatalf("frame 1 left half should be transparent after background disposal, got alpha=%d", a)
	}
}

func TestAnimatedGIFInBoundsPassesThrough(t *testing.T) {
	raw := makeAnimatedGIF(t, 800, 100, 4)
	opt, err := Optimize(raw, "gif")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(opt.Data, raw) {
		t.Fatal("in-bounds animated GIF must pass through byte-identical")
	}
	if opt.Width != 800 || opt.Height != 100 {
		t.Fatalf("dimensions misreported: %dx%d", opt.Width, opt.Height)
	}
}

func TestAnimatedGIFOverWideIsResizedAndStaysAnimated(t *testing.T) {
	raw := makeAnimatedGIF(t, 2400, 200, 4)
	opt, err := Optimize(raw, "gif")
	if err != nil {
		t.Fatal(err)
	}
	if opt.Ext != "gif" {
		t.Fatalf("animated GIF changed format to %q", opt.Ext)
	}
	g, err := gif.DecodeAll(bytes.NewReader(opt.Data))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 4 {
		t.Fatalf("frame count = %d, want 4", len(g.Image))
	}
	if bytes.Equal(opt.Data, raw) {
		// keep-original guard fired; acceptable, but dims must reflect the original then.
		if opt.Width != 2400 {
			t.Fatalf("kept original but width = %d", opt.Width)
		}
		return
	}
	if opt.Width != MaxImageWidth || g.Image[0].Bounds().Dx() != MaxImageWidth {
		t.Fatalf("width = %d / frame %d, want %d", opt.Width, g.Image[0].Bounds().Dx(), MaxImageWidth)
	}
	if len(opt.Data) >= len(raw) {
		t.Fatalf("re-encode (%d) not smaller than original (%d)", len(opt.Data), len(raw))
	}
}

func TestStaticGIFJoinsRasterRule(t *testing.T) {
	raw := makeAnimatedGIF(t, 300, 100, 1)
	opt, err := Optimize(raw, "gif")
	if err != nil {
		t.Fatal(err)
	}
	// Flat-color art: quantized PNG or the original may win, but the result
	// must never be larger, and never JPEG-with-alpha nonsense.
	if len(opt.Data) > len(raw) {
		t.Fatalf("output (%d) larger than original (%d)", len(opt.Data), len(raw))
	}
	if _, _, err := image.Decode(bytes.NewReader(opt.Data)); err != nil {
		t.Fatalf("output not decodable: %v", err)
	}
}
