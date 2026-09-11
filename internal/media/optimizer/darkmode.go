package optimizer

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"math"

	"github.com/disintegration/imaging"
)

// Dark-mode classification and repair (DARK-MODE-SPEC D4).
//
// Three dark-mode client behaviours were observed across the template 29 matrix
// (2026-09-10), and ALL THREE leave images alone: a dark client never inverts an image (the
// one exception is Gmail Android, which recolours SMALL dark-on-transparent glyphs -- so a
// glyph must read in both the recoloured and the untouched case, which is what an opaque
// light disc guarantees). The consequences are mechanical:
//
//   - a dark mark on a transparent ground disappears on a dark background;
//   - an opaque white rectangle stays a white slab on a dark background;
//   - a ring glyph whose interior is transparent loses its enclosed shape.
//
// Those precepts existed only as runbook prose, and the three assets that broke template 29
// were uploaded outside any spec chain. Classify puts a verdict on every raster upload, and
// Repair fixes the two classes that have a safe, unambiguous repair. The other two are
// warnings: keying a coloured logo off a white ground would produce the very shape this
// file exists to eliminate, and a dark-on-transparent artwork needs an author's decision
// (recolour, halo, or a light variant), not a guess.

// ClassifierVersion stamps every verdict. The lint helper on the integrations side
// (lib/listmonk-media-lint.ts) compares against it to find stale rows, so the two constants
// MUST be bumped together -- see the listmonk runbook's dark-mode bullet.
const ClassifierVersion = 3

// Image classes, in evaluation order.
const (
	ClassPhoto             = "photo"
	ClassAnimated          = "animated"
	ClassIconRing          = "icon-ring"
	ClassMonoOnWhite       = "mono-on-white"
	ClassWhiteBackground   = "white-background"
	ClassMonoOnTransparent = "mono-on-transparent"
	ClassDarkOnTransparent = "dark-on-transparent"
	ClassOK                = "ok"
)

// Warning codes carried on a Verdict. They are the class name for the two classes that
// warn, so a consumer never has to map one to the other.
const (
	WarnWhiteBackground   = ClassWhiteBackground
	WarnDarkOnTransparent = ClassDarkOnTransparent
)

// Classification thresholds. Every luminance here is gamma-space luma (see lumaAt).
const (
	// monochrome: this share of opaque pixels must be near-grey. A distinct-colour count
	// cannot stand in for it -- anti-aliasing gives the live exclusively_on_curated_logo.png
	// and curated_logo3.png 32 quantised colours each.
	monoChannelSpread = 16
	monoFraction      = 0.99

	// photo: distinct colours, quantised 5 bits per channel (the measure that reads those
	// two logos as 32).
	photoDistinctColors = 64

	// A pixel at or above this luma is "white" for the border-whiteness and ink measures.
	nearWhite = 0.95

	darkInk = 0.35 // ink at or below this is dark
	// icon-ring repair geometry (the Curated convention, measured on the live
	// social-curated-facebook.png: white disc at the full 104px diameter, ring outer radius
	// 47 -- inset 5px, ~10% of the disc radius). The margin OUTSIDE the ring is what keeps
	// the ring visible on a dark ground; a ring at the canvas edge merges into it, which is
	// what the interior-only fill produced for social-curated-email.png on 2026-09-11.
	ringInset     = 0.10
	iconMaxDim    = 256  // icon-ring applies to small glyphs only
	iconEnclosed  = 0.20 // enclosed transparent area, as a share of the opaque bounding box
	borderWhiteFr = 0.90 // share of the 2px outer ring that must be white
	borderRing    = 2
	// dark-on-transparent is decided by WHERE the ink meets transparency, not by how much
	// transparency there is (a transparency fraction cannot tell a padded disc from a bare
	// glyph: the live 104px discs measure 0.215, a 96px disc in a 104px box 0.33). The
	// silhouette rim -- opaque pixels with a transparent 4-neighbour or the image edge -- is
	// what a dark client's background touches: a light rim is a backing (a disc), a dark rim
	// is bare ink. A dark rim on a SOLID shape that CARRIES LIGHT INK (a dark-coloured disc
	// with a light glyph, the Liyora set) is not a defect either: the shape survives
	// untouched and is not recolored (hazard 58), and the glyph still reads on a dark ground.
	// A solid dark shape with no light ink is just a bigger bare mark.
	rimDarkFr    = 0.50  // share of the silhouette rim that is dark ink
	solidFill    = 0.60  // opaque share of the bounding box at or above which a shape is solid
	lightInkFr   = 0.005 // share of opaque pixels that must be near-white for a solid shape to count as carrying a glyph (a bare dark mark has none; the live minimum is 0.051)
	opaqueEnough = 0.01  // "no alpha" tolerates less than this much transparency

	// alphaOpaque is the 16-bit alpha at or above which a pixel counts as opaque.
	alphaOpaque = 0x8000
)

// repairInk is the grey every mono-on-white mark is recoloured to: the 2026-09-07
// curated_logo3_grey recipe, and the accepted Curated-furniture aesthetic. It reads as a
// mid-tone on both a white and a dark ground, which is the whole point -- a #000 mark keyed
// to transparency would simply move the defect to the dark clients.
var repairInk = color.RGBA{R: 0x77, G: 0x77, B: 0x77, A: 0xff}

// Verdict is the outcome of Classify. It is stamped into a media row's meta.darkmode and
// returned on the upload response.
type Verdict struct {
	Class    string   `json:"class"`
	Fixed    bool     `json:"fixed"`
	Warnings []string `json:"warnings"`
	Version  int      `json:"version"`
}

// lumaAt is Rec. 709 luma on the sRGB-ENCODED channel values.
//
// NOT WCAG's linear relative luminance, deliberately: the linearised measure reads the
// already-repaired curated_logo3_grey.png (#777777) as 0.184, i.e. "dark", which would make
// the Inspect pre-flight block every Curated template. Gamma-space luma reads it 0.467 -- a
// mid-tone, which is what the eye sees. The admin's dark simulation (darkSim.js) uses the
// same formula so "dark" means one thing on both sides.
func lumaAt(r, g, b uint32) float64 {
	return (0.2126*float64(r>>8) + 0.7152*float64(g>>8) + 0.0722*float64(b>>8)) / 255
}

// straightAlpha un-premultiplies a pixel's 16-bit channels. image.Image.At().RGBA() returns
// alpha-premultiplied values, so an anti-aliased white pixel at 60% alpha reads as luma 0.6
// -- "ink" -- unless the alpha is divided back out. Every luma, spread and colour measure
// in this file reads through it, so a soft edge is measured by its colour, not its coverage.
// Callers only pass opaque pixels (alpha >= alphaOpaque), so a is never zero.
func straightAlpha(r, g, b, a uint32) (uint32, uint32, uint32) {
	return r * 0xffff / a, g * 0xffff / a, b * 0xffff / a
}

// stats holds every measurement Classify derives, computed in one pass over the image.
type stats struct {
	w, h           int
	total          int
	opaque         int
	transparent    int
	monoOpaque     int
	inkLuma        float64 // mean luma over opaque pixels that are NOT near-white
	distinctColors int
	borderWhite    float64
	enclosedFrac   float64 // enclosed transparent area / opaque bounding-box area
	bboxW, bboxH   int
	transparentFr  float64
	monoFr         float64
	rimDarkFr      float64 // share of the silhouette rim (opaque pixels touching transparency) that is dark ink
	fillFr         float64 // opaque pixels / opaque bounding-box area
	lightFr        float64 // share of opaque pixels that are near-white
}

func measure(img image.Image) stats {
	b := img.Bounds()
	s := stats{w: b.Dx(), h: b.Dy()}
	s.total = s.w * s.h
	if s.total == 0 {
		return s
	}

	var (
		inkSum     float64
		inkN       int
		lightN     int
		colors     = make(map[uint32]struct{})
		trans      = make([]bool, s.total)
		minX, minY = s.w, s.h
		maxX, maxY = -1, -1
	)

	for y := 0; y < s.h; y++ {
		for x := 0; x < s.w; x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a < alphaOpaque {
				trans[y*s.w+x] = true
				s.transparent++
				continue
			}

			s.opaque++
			r, g, bl = straightAlpha(r, g, bl, a)
			if x < minX {
				minX = x
			}
			if x > maxX {
				maxX = x
			}
			if y < minY {
				minY = y
			}
			if y > maxY {
				maxY = y
			}

			r8, g8, b8 := r>>8, g>>8, bl>>8
			mx, mn := r8, r8
			if g8 > mx {
				mx = g8
			}
			if b8 > mx {
				mx = b8
			}
			if g8 < mn {
				mn = g8
			}
			if b8 < mn {
				mn = b8
			}
			if mx-mn <= monoChannelSpread {
				s.monoOpaque++
			}

			if l := lumaAt(r, g, bl); l < nearWhite {
				inkSum += l
				inkN++
			} else {
				lightN++
			}

			// 5 bits per channel -- the quantisation under which the two live Curated
			// logos read as 32 colours.
			colors[(r8>>3)<<10|(g8>>3)<<5|(b8>>3)] = struct{}{}
		}
	}

	s.distinctColors = len(colors)
	s.transparentFr = float64(s.transparent) / float64(s.total)
	if s.opaque > 0 {
		s.monoFr = float64(s.monoOpaque) / float64(s.opaque)
		s.lightFr = float64(lightN) / float64(s.opaque)
	}

	// "Ink" is the mark, not the ground: the mean over opaque pixels BELOW near-white. A
	// mean over every opaque pixel is dominated by the white ground (the live
	// curated_logo3.png reads 0.672 that way -- "light" -- while its mark is 0.052), which
	// would make mono-on-white unreachable for exactly the files it exists for.
	switch {
	case inkN > 0:
		s.inkLuma = inkSum / float64(inkN)
	case s.opaque > 0:
		s.inkLuma = 1 // entirely near-white: no dark ink at all
	}

	if maxX >= 0 {
		s.bboxW, s.bboxH = maxX-minX+1, maxY-minY+1
		s.enclosedFrac = float64(enclosedTransparent(trans, s.w, s.h)) / float64(s.bboxW*s.bboxH)
		s.fillFr = float64(s.opaque) / float64(s.bboxW*s.bboxH)
		s.rimDarkFr = rimDarkness(img, trans)
	}

	s.borderWhite = borderWhiteness(img)
	return s
}

// rimDarkness is the share of the silhouette rim that is dark ink. The rim is every opaque
// pixel with a transparent 4-neighbour (the image edge counts as transparent).
func rimDarkness(img image.Image, trans []bool) float64 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	isTrans := func(x, y int) bool {
		return x < 0 || y < 0 || x >= w || y >= h || trans[y*w+x]
	}

	rim, dark := 0, 0
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if trans[y*w+x] {
				continue
			}
			if !isTrans(x-1, y) && !isTrans(x+1, y) && !isTrans(x, y-1) && !isTrans(x, y+1) {
				continue
			}
			rim++
			if lumaAt(straightAlpha(img.At(b.Min.X+x, b.Min.Y+y).RGBA())) <= darkInk {
				dark++
			}
		}
	}
	if rim == 0 {
		return 0
	}
	return float64(dark) / float64(rim)
}

// borderWhiteness is the share of the borderRing-px outer ring whose pixels are opaque and
// near-white.
func borderWhiteness(img image.Image) float64 {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	var ring, white int
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if x >= borderRing && w-1-x >= borderRing && y >= borderRing && h-1-y >= borderRing {
				continue
			}
			ring++
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a >= alphaOpaque && lumaAt(straightAlpha(r, g, bl, a)) >= nearWhite {
				white++
			}
		}
	}
	if ring == 0 {
		return 0
	}
	return float64(white) / float64(ring)
}

// enclosedTransparent counts transparent pixels NOT reachable from the image border --
// the interior of a ring glyph, as opposed to the transparency around it.
func enclosedTransparent(trans []bool, w, h int) int {
	if w == 0 || h == 0 {
		return 0
	}

	seen := make([]bool, len(trans))
	stack := make([]int, 0, len(trans)/4)
	push := func(i int) {
		if i >= 0 && i < len(trans) && trans[i] && !seen[i] {
			seen[i] = true
			stack = append(stack, i)
		}
	}

	for x := 0; x < w; x++ {
		push(x)
		push((h-1)*w + x)
	}
	for y := 0; y < h; y++ {
		push(y * w)
		push(y*w + w - 1)
	}

	for len(stack) > 0 {
		i := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		x, y := i%w, i/w
		if x > 0 {
			push(i - 1)
		}
		if x < w-1 {
			push(i + 1)
		}
		if y > 0 {
			push(i - w)
		}
		if y < h-1 {
			push(i + w)
		}
	}

	n := 0
	for i, t := range trans {
		if t && !seen[i] {
			n++
		}
	}
	return n
}

// Classify returns the dark-mode verdict for a decoded still image. `ext` is the upload's
// extension: a JPEG is photographic by construction (lossy, no alpha), which short-circuits
// the pixel measures for the common case.
//
// Classes are evaluated in the documented order; the first match wins.
func Classify(img image.Image, ext string) Verdict {
	v := Verdict{Class: ClassOK, Warnings: []string{}, Version: ClassifierVersion}
	if img == nil {
		return v
	}

	s := measure(img)
	mono := s.opaque > 0 && s.monoFr >= monoFraction
	isJPEG := ext == "jpg" || ext == "jpeg"
	hasAlpha := s.transparentFr >= opaqueEnough
	dark := s.inkLuma <= darkInk
	whiteBorder := s.borderWhite >= borderWhiteFr
	// bareRim: the silhouette meets transparency with dark ink. backedDisc: a solid shape
	// carrying light ink -- a dark-coloured disc holding a light glyph, which is not bare.
	bareRim := s.rimDarkFr >= rimDarkFr
	backedDisc := s.fillFr >= solidFill && s.lightFr >= lightInkFr

	switch {
	case isJPEG || (!mono && s.distinctColors >= photoDistinctColors):
		v.Class = ClassPhoto

	case hasAlpha && s.w <= iconMaxDim && s.h <= iconMaxDim && mono && dark && s.enclosedFrac >= iconEnclosed:
		v.Class = ClassIconRing

	case !hasAlpha && whiteBorder && mono && dark:
		v.Class = ClassMonoOnWhite

	case !hasAlpha && whiteBorder && !mono:
		v.Class = ClassWhiteBackground
		v.Warnings = append(v.Warnings, WarnWhiteBackground)

	case hasAlpha && dark && mono && bareRim && !backedDisc:
		v.Class = ClassMonoOnTransparent

	case hasAlpha && dark && bareRim && !backedDisc:
		v.Class = ClassDarkOnTransparent
		v.Warnings = append(v.Warnings, WarnDarkOnTransparent)
	}

	return v
}

// ClassifyAnimated is the verdict for an animated GIF. Animations are refused, never
// repaired (the existing budget rule), so they are classified and left alone.
func ClassifyAnimated() Verdict {
	return Verdict{Class: ClassAnimated, Warnings: []string{}, Version: ClassifierVersion}
}

// Repairable reports whether a class has an automatic pixel repair.
func Repairable(class string) bool {
	return class == ClassIconRing || class == ClassMonoOnWhite || class == ClassMonoOnTransparent
}

// Repair applies the class's fix and reports whether pixels changed. An image whose class
// has no fix is returned as-is, so Repair is idempotent by construction: a repaired image
// classifies `ok`, and `ok` has no fix.
func Repair(img image.Image, v Verdict) (image.Image, bool) {
	switch v.Class {
	case ClassIconRing:
		return discBacked(img), true
	case ClassMonoOnWhite:
		return keyWhiteToAlpha(img), true
	case ClassMonoOnTransparent:
		return recolorInk(img), true
	}
	return img, false
}

// recolorInk flattens a monochrome mark's RGB to repairInk and leaves its alpha exactly as
// it is: the same recipe keyWhiteToAlpha ends with, for art that already carries its own
// coverage. A black wordmark on transparency vanishes on every inverting dark client; the
// mid grey reads on a white page and a dark one alike (the curated_logo3_grey precedent).
// Coloured art is never recoloured -- that stays an author's decision (dark-on-transparent).
func recolorInk(img image.Image) image.Image {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			c := color.NRGBAModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.NRGBA)
			if c.A == 0 {
				continue
			}
			out.SetNRGBA(x, y, color.NRGBA{R: repairInk.R, G: repairInk.G, B: repairInk.B, A: c.A})
		}
	}
	return out
}

// discBacked rebuilds a ring glyph in the Curated geometry: an opaque white disc at the
// canvas's full diameter with the glyph scaled so its ring sits ringInset inside the disc's
// edge, centred. The disc is what makes the glyph legible in BOTH dark-client cases:
// untouched (black on white) and recoloured by Gmail Android (white on white disc is still
// a disc). The margin outside the ring keeps the ring itself visible on a dark ground.
// The canvas size is unchanged, so the template's width attribute still fits.
//
// A glyph already smaller than the inset disc is not upscaled -- it is only centred.
func discBacked(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	if w == 0 || h == 0 {
		return out
	}

	// Two bounding boxes: the OPAQUE one is the glyph's geometry (its half-extent is the
	// ring's outer radius, the number the inset is computed from); the COVERAGE one (any
	// alpha at all) is what gets cropped, so the anti-aliased fringe outside the opaque
	// edge rides along instead of being cut off and hardening the ring's outer edge.
	var opaque, covered bbox
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			_, _, _, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a > 0 {
				covered.add(x, y)
			}
			if a >= alphaOpaque {
				opaque.add(x, y)
			}
		}
	}
	if opaque.empty() {
		return out
	}

	disc := float64(min(w, h)) / 2
	margin := math.Round(disc * ringInset)
	ringR := float64(max(opaque.w(), opaque.h())) / 2
	scale := (disc - margin) / ringR
	if scale > 1 {
		scale = 1
	}

	glyph := imaging.Crop(img, image.Rect(b.Min.X+covered.minX, b.Min.Y+covered.minY, b.Min.X+covered.maxX+1, b.Min.Y+covered.maxY+1))
	gw := int(math.Round(float64(covered.w()) * scale))
	gh := int(math.Round(float64(covered.h()) * scale))
	if gw < 1 {
		gw = 1
	}
	if gh < 1 {
		gh = 1
	}
	if scale < 1 {
		glyph = imaging.Resize(glyph, gw, gh, imaging.Lanczos)
	}

	// The white disc, anti-aliased at its edge by signed distance.
	cx, cy := float64(w)/2, float64(h)/2
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			cov := disc - d + 0.5
			if cov <= 0 {
				continue
			}
			if cov > 1 {
				cov = 1
			}
			out.SetNRGBA(x, y, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: uint8(cov*255 + 0.5)})
		}
	}

	off := image.Pt((w-gw)/2, (h-gh)/2)
	draw.Draw(out, image.Rect(off.X, off.Y, off.X+gw, off.Y+gh), glyph, glyph.Bounds().Min, draw.Over)
	return out
}

// bbox accumulates a pixel bounding box; empty until the first add.
type bbox struct {
	minX, minY, maxX, maxY int
	set                    bool
}

func (b *bbox) add(x, y int) {
	if !b.set {
		b.minX, b.minY, b.maxX, b.maxY, b.set = x, y, x, y, true
		return
	}
	b.minX, b.maxX = min(b.minX, x), max(b.maxX, x)
	b.minY, b.maxY = min(b.minY, y), max(b.maxY, y)
}

func (b *bbox) empty() bool { return !b.set }
func (b *bbox) w() int      { return b.maxX - b.minX + 1 }
func (b *bbox) h() int      { return b.maxY - b.minY + 1 }

// keyWhiteToAlpha turns a dark mark on an opaque white ground into a #777777 mark on
// transparency.
//
// Alpha is COVERAGE (1 - luma), not a hard key: a 0.95 threshold leaves a jagged opaque
// fringe on every anti-aliased edge, which is worse than the slab it replaces. The RGB is
// flattened to repairInk so the mark reads on a white ground and on a dark one alike.
func keyWhiteToAlpha(img image.Image) image.Image {
	b := img.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			r, g, bl, a := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			if a < alphaOpaque {
				continue // already transparent; leave it so
			}
			cov := 1 - lumaAt(r, g, bl)
			if cov < 0 {
				cov = 0
			}
			out.SetNRGBA(x, y, color.NRGBA{
				R: repairInk.R, G: repairInk.G, B: repairInk.B,
				A: uint8(cov*255 + 0.5),
			})
		}
	}
	return out
}

// EncodePNG re-encodes a repaired image. Repairs always produce PNG: both of them create or
// preserve an alpha channel, which JPEG cannot carry.
func EncodePNG(img image.Image) ([]byte, error) {
	var out bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ClassifyRaw is the single entry point the upload/reprocess paths use: it decodes the
// bytes, routes an animated GIF to ClassifyAnimated, and classifies everything else.
// Keeping the animated split here means the handler and the tests cannot disagree about
// what an animation classifies as.
func ClassifyRaw(raw []byte, ext string) (image.Image, Verdict, error) {
	if ext == "gif" {
		if g, err := gif.DecodeAll(bytes.NewReader(raw)); err == nil && len(g.Image) > 1 {
			return nil, ClassifyAnimated(), nil
		}
	}

	img, err := DecodeStill(raw)
	if err != nil {
		return nil, Verdict{}, err
	}
	return img, Classify(img, ext), nil
}

// DecodeStill decodes an upload's bytes for classification.
func DecodeStill(raw []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	return img, err
}
