package optimizer

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

// Dark-mode classifier and repairs (DARK-MODE-SPEC D4, invariants I4a / I4b / I4d).
//
// The fixtures under testdata/ are the LIVE files from email.curatedfor.you/uploads/ --
// every asset named in the spec's origin plus the two controls (the already-repaired grey
// wordmark, and the social icon that already carries its white disc). Classifying the real
// bytes is the point: each threshold was derived from these files, so a drifting measure
// fails here rather than on a send.

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return raw
}

func extOf(name string) string {
	switch filepath.Ext(name) {
	case ".jpg", ".jpeg":
		return "jpg"
	case ".gif":
		return "gif"
	}
	return "png"
}

// colouredLogoOnWhite: a few flat brand colours on an opaque white ground. Monochrome fails
// (it is coloured) and the distinct-colour count stays well under the photo threshold, so it
// is the white-background case -- warn, never key.
func colouredLogoOnWhite() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 300, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 300; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
		}
	}
	marks := []color.NRGBA{
		{R: 0x6f, G: 0x09, B: 0x36, A: 0xff},
		{R: 0x1d, G: 0x6f, B: 0x3a, A: 0xff},
		{R: 0x2b, G: 0x3a, B: 0x9f, A: 0xff},
	}
	for i, c := range marks {
		for y := 30; y < 90; y++ {
			for x := 40 + i*80; x < 100+i*80; x++ {
				img.SetNRGBA(x, y, c)
			}
		}
	}
	return img
}

// padTransparent decodes a PNG fixture and centres it on a transparent canvas `pad` px
// larger on every side.
func padTransparent(t *testing.T, raw []byte, pad int) image.Image {
	t.Helper()
	src, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	b := src.Bounds()
	out := image.NewNRGBA(image.Rect(0, 0, b.Dx()+2*pad, b.Dy()+2*pad))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x+pad, y+pad, src.At(b.Min.X+x, b.Min.Y+y))
		}
	}
	return out
}

// darkDiscWithHairline: a 104px near-black disc carrying a single one-pixel-wide light
// vertical stroke -- the thinnest glyph a disc can hold.
func darkDiscWithHairline() image.Image {
	const n, r = 104, 48
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			dx, dy := float64(x)-n/2+0.5, float64(y)-n/2+0.5
			if dx*dx+dy*dy > r*r {
				continue
			}
			c := color.NRGBA{R: 0x1a, G: 0x1a, B: 0x1a, A: 0xff}
			if x == n/2 && y > n/2-20 && y < n/2+20 {
				c = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// thickDarkPlus: a near-black plus sign whose two 60px arms fill 84% of a 100px box -- a
// bare glyph with LESS than 30% transparency.
func thickDarkPlus() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 100, 100))
	for y := 0; y < 100; y++ {
		for x := 0; x < 100; x++ {
			if (x >= 20 && x < 80) || (y >= 20 && y < 80) {
				img.SetNRGBA(x, y, color.NRGBA{R: 0x10, G: 0x10, B: 0x10, A: 0xff})
			}
		}
	}
	return img
}

// studioPhoto: a noisy subject on a white studio ground -- the guard fixture for I4d. It
// must classify `photo` (>= 64 distinct colours, not monochrome) and therefore never be
// keyed, even though its border is pure white.
func studioPhoto() image.Image {
	rng := rand.New(rand.NewSource(7))
	img := image.NewNRGBA(image.Rect(0, 0, 300, 200))
	for y := 0; y < 200; y++ {
		for x := 0; x < 300; x++ {
			c := color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
			if x > 60 && x < 240 && y > 40 && y < 160 {
				c = color.NRGBA{
					R: uint8(rng.Intn(256)), G: uint8(rng.Intn(256)), B: uint8(rng.Intn(256)), A: 0xff,
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

func animatedGIF(t *testing.T) []byte {
	t.Helper()
	pal := color.Palette{color.Black, color.White}
	g := &gif.GIF{}
	for i := 0; i < 3; i++ {
		fr := image.NewPaletted(image.Rect(0, 0, 32, 32), pal)
		for p := range fr.Pix {
			fr.Pix[p] = uint8((p + i) % 2)
		}
		g.Image = append(g.Image, fr)
		g.Delay = append(g.Delay, 10)
	}
	var out bytes.Buffer
	if err := gif.EncodeAll(&out, g); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func encodeSyntheticPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// ---- I4a ------------------------------------------------------------------------------

func TestClassifyFixtures(t *testing.T) {
	live := []struct {
		file string
		want string
		why  string
	}{
		{"social-curated-email.png", ClassIconRing,
			"black ring + glyph on a transparent interior; the envelope vanished on nine dark clients"},
		{"shala_hero.png", ClassDarkOnTransparent,
			"dark ink on transparency; invisible on every inverting dark client"},
		{"exclusively_on_curated_logo.png", ClassMonoOnWhite,
			"opaque white, no alpha; a white slab on a dark ground"},
		{"curated_logo3.png", ClassMonoOnWhite,
			"the same defect, fixed by hand on 2026-09-07"},
		{"curated_logo3_grey.png", ClassOK,
			"the 2026-09-07 repair: #777 mark on transparency, a mid-tone that reads either way"},
		{"shala_hero2.jpg", ClassPhoto, "photographic JPEG"},
		{"social-curated-facebook.png", ClassOK, "already carries the opaque white disc"},
		{"social-facebook.png", ClassOK,
			"Liyora: a dark-coloured SOLID disc carrying a light glyph; a dark rim on such a shape is not bare ink (hazard 58)"},
	}

	for _, c := range live {
		t.Run(c.file, func(t *testing.T) {
			raw := fixture(t, c.file)
			_, v, err := ClassifyRaw(raw, extOf(c.file))
			if err != nil {
				t.Fatalf("classify: %v", err)
			}
			if v.Class != c.want {
				t.Fatalf("%s classified %q, want %q (%s)", c.file, v.Class, c.want, c.why)
			}
			if v.Version != ClassifierVersion {
				t.Fatalf("verdict version %d, want %d", v.Version, ClassifierVersion)
			}
		})
	}

	// The discriminator is the silhouette rim and solidity, NOT the transparency fraction: a
	// padded disc (more transparency than the 104px originals) must still be `ok`, light-disc
	// and dark-disc alike, while a bare glyph is flagged however little padding it has.
	for _, c := range []struct{ file, why string }{
		{"social-curated-facebook.png", "light disc, padded: the rim is light, so the ink is backed"},
		{"social-facebook.png", "dark disc, padded: the rim is dark but the shape is solid and carries a light glyph"},
	} {
		t.Run("padded "+c.file, func(t *testing.T) {
			padded := padTransparent(t, fixture(t, c.file), 60)
			if s := measure(padded); s.transparentFr < 0.5 {
				t.Fatalf("fixture not padded enough to be a test: transparent %.2f", s.transparentFr)
			}
			_, v, err := ClassifyRaw(encodeSyntheticPNG(t, padded), "png")
			if err != nil {
				t.Fatal(err)
			}
			if v.Class != ClassOK {
				t.Fatalf("classified %q, want %q (%s)", v.Class, ClassOK, c.why)
			}
		})
	}

	t.Run("dark disc with a hairline glyph", func(t *testing.T) {
		// The thinnest glyph a dark disc can carry: a one-pixel light stroke. It must still
		// count as a disc carrying a glyph, so the light-ink floor sits far under the live
		// minimum (0.051, the Liyora Facebook disc) but above zero.
		img := darkDiscWithHairline()
		if s := measure(img); s.lightFr >= 0.02 || s.lightFr < lightInkFr {
			t.Fatalf("fixture light-ink share %.4f is not in the band this test exists for", s.lightFr)
		}
		_, v, err := ClassifyRaw(encodeSyntheticPNG(t, img), "png")
		if err != nil {
			t.Fatal(err)
		}
		if v.Class != ClassOK {
			t.Fatalf("classified %q, want %q", v.Class, ClassOK)
		}
	})

	t.Run("bare glyph with little padding", func(t *testing.T) {
		// A thick dark plus sign filling most of its box: under 30% transparency, which the
		// retired transparency-fraction rule would have passed. It is as solid as a disc, but
		// carries no light ink -- it IS the mark -- so it is bare ink.
		_, v, err := ClassifyRaw(encodeSyntheticPNG(t, thickDarkPlus()), "png")
		if err != nil {
			t.Fatal(err)
		}
		if v.Class != ClassDarkOnTransparent {
			t.Fatalf("classified %q, want %q", v.Class, ClassDarkOnTransparent)
		}
	})

	t.Run("coloured logo on white", func(t *testing.T) {
		_, v, err := ClassifyRaw(encodeSyntheticPNG(t, colouredLogoOnWhite()), "png")
		if err != nil {
			t.Fatal(err)
		}
		if v.Class != ClassWhiteBackground {
			t.Fatalf("classified %q, want %q", v.Class, ClassWhiteBackground)
		}
		if len(v.Warnings) != 1 || v.Warnings[0] != WarnWhiteBackground {
			t.Fatalf("warnings %v, want [%s]", v.Warnings, WarnWhiteBackground)
		}
	})

	t.Run("synthetic studio photo PNG", func(t *testing.T) {
		_, v, err := ClassifyRaw(encodeSyntheticPNG(t, studioPhoto()), "png")
		if err != nil {
			t.Fatal(err)
		}
		if v.Class != ClassPhoto {
			t.Fatalf("classified %q, want %q", v.Class, ClassPhoto)
		}
	})

	t.Run("animated gif", func(t *testing.T) {
		_, v, err := ClassifyRaw(animatedGIF(t), "gif")
		if err != nil {
			t.Fatal(err)
		}
		if v.Class != ClassAnimated {
			t.Fatalf("classified %q, want %q", v.Class, ClassAnimated)
		}
		if Repairable(v.Class) {
			t.Fatal("an animation must never be repairable (refuse, never repair)")
		}
	})
}

func TestClassifyWarningCodes(t *testing.T) {
	_, v, err := ClassifyRaw(fixture(t, "shala_hero.png"), "png")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Warnings) != 1 || v.Warnings[0] != WarnDarkOnTransparent {
		t.Fatalf("warnings %v, want [%s]", v.Warnings, WarnDarkOnTransparent)
	}
	if Repairable(v.Class) {
		t.Fatal("dark-on-transparent has no automatic repair -- it is an author's decision")
	}

	// The classes with no defect carry no warnings at all.
	for _, f := range []string{"curated_logo3_grey.png", "social-curated-facebook.png", "shala_hero2.jpg"} {
		_, ok, err := ClassifyRaw(fixture(t, f), extOf(f))
		if err != nil {
			t.Fatal(err)
		}
		if len(ok.Warnings) != 0 {
			t.Fatalf("%s carries warnings %v", f, ok.Warnings)
		}
	}
}

// ---- I4b ------------------------------------------------------------------------------

func pixelsEqual(a, b image.Image) bool {
	if a.Bounds() != b.Bounds() {
		return false
	}
	bd := a.Bounds()
	for y := bd.Min.Y; y < bd.Max.Y; y++ {
		for x := bd.Min.X; x < bd.Max.X; x++ {
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			// Fully transparent pixels carry no meaningful colour.
			if a1 == 0 && a2 == 0 {
				continue
			}
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				return false
			}
		}
	}
	return true
}

func TestRepairIdempotent(t *testing.T) {
	for _, f := range []string{"social-curated-email.png", "exclusively_on_curated_logo.png", "curated_logo3.png"} {
		t.Run(f, func(t *testing.T) {
			img, v, err := ClassifyRaw(fixture(t, f), extOf(f))
			if err != nil {
				t.Fatal(err)
			}
			if !Repairable(v.Class) {
				t.Fatalf("%s classified %q, which has no repair", f, v.Class)
			}

			once, changed := Repair(img, v)
			if !changed {
				t.Fatal("repair reported no change")
			}

			// The repaired image must classify clean: that is what makes the second
			// repair a no-op, and what the D6 pre-flight checks for.
			after := Classify(once, "png")
			if after.Class != ClassOK {
				t.Fatalf("repaired %s classifies %q, want %q", f, after.Class, ClassOK)
			}
			if len(after.Warnings) != 0 {
				t.Fatalf("repaired %s still warns: %v", f, after.Warnings)
			}

			twice, changedAgain := Repair(once, after)
			if changedAgain {
				t.Fatal("the second repair changed pixels")
			}
			if !pixelsEqual(once, twice) {
				t.Fatal("Repair(Repair(x)) != Repair(x)")
			}

			// And byte-for-byte through the PNG encoder, which is what actually gets
			// stored.
			b1, err := EncodePNG(once)
			if err != nil {
				t.Fatal(err)
			}
			b2, err := EncodePNG(twice)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(b1, b2) {
				t.Fatal("re-encoded repaired bytes differ")
			}
		})
	}
}

// ringOuterRadius is the distance from the canvas centre to the first dark opaque pixel on
// the horizontal centre row -- the ring's outer radius, the number the Curated geometry is
// pinned on.
func ringOuterRadius(img image.Image) float64 {
	b := img.Bounds()
	y := b.Min.Y + b.Dy()/2
	for x := 0; x < b.Dx(); x++ {
		r, g, bl, a := img.At(b.Min.X+x, y).RGBA()
		if a >= alphaOpaque && lumaAt(straightAlpha(r, g, bl, a)) <= darkInk {
			return float64(b.Dx())/2 - float64(x)
		}
	}
	return 0
}

func TestRepairIconRingMatchesTheCuratedGeometry(t *testing.T) {
	img, v, err := ClassifyRaw(fixture(t, "social-curated-email.png"), "png")
	if err != nil {
		t.Fatal(err)
	}
	if v.Class != ClassIconRing {
		t.Fatalf("fixture classifies %q", v.Class)
	}
	out, changed := Repair(img, v)
	if !changed {
		t.Fatal("no repair applied")
	}

	// Same canvas, so the template's width attribute still fits.
	if out.Bounds().Dx() != img.Bounds().Dx() || out.Bounds().Dy() != img.Bounds().Dy() {
		t.Fatalf("canvas changed: %v -> %v", img.Bounds(), out.Bounds())
	}

	// The ring sits at the Facebook icon's radius: the geometry every Curated glyph shares.
	ref, err := png.Decode(bytes.NewReader(fixture(t, "social-curated-facebook.png")))
	if err != nil {
		t.Fatal(err)
	}
	want, got := ringOuterRadius(ref), ringOuterRadius(out)
	if math.Abs(want-got) > 1 {
		t.Fatalf("ring outer radius %.1f, want %.1f (facebook) +-1", got, want)
	}

	// Outside the ring is opaque white (the margin), the centre is opaque white (the
	// interior), and the corner is transparent (a disc, not a square).
	b := out.Bounds()
	edgeX := b.Min.X + int(float64(b.Dx())/2-want) - 2
	if r, g, bl, a := out.At(edgeX, b.Min.Y+b.Dy()/2).RGBA(); a != 0xffff || r != 0xffff || g != 0xffff || bl != 0xffff {
		t.Fatalf("margin outside the ring is %v,%v,%v,%v -- want opaque white", r, g, bl, a)
	}
	if r, g, bl, a := out.At(b.Min.X+b.Dx()/2, b.Min.Y+b.Dy()/2).RGBA(); a != 0xffff || r != 0xffff || g != 0xffff || bl != 0xffff {
		t.Fatalf("ring interior is %v,%v,%v,%v -- want opaque white", r, g, bl, a)
	}
	if _, _, _, corner := out.At(b.Min.X, b.Min.Y).RGBA(); corner >= 0x8000 {
		t.Fatalf("the corner became opaque (alpha %v)", corner)
	}

	// And it now measures like the Facebook icon: a light rim, solid, `ok`.
	if s := measure(out); s.rimDarkFr > 0.05 {
		t.Fatalf("repaired rim is %.2f dark -- the outer circle would merge into a dark ground", s.rimDarkFr)
	}
	if after := Classify(out, "png"); after.Class != ClassOK {
		t.Fatalf("repaired glyph classifies %q, want ok", after.Class)
	}
}

func TestRepairMonoOnWhiteIsCoverageNotAKey(t *testing.T) {
	img, v, err := ClassifyRaw(fixture(t, "curated_logo3.png"), "png")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := Repair(img, v)

	b := out.Bounds()
	var opaque, partial int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := out.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			if a == 0xffff {
				opaque++
				// Every opaque mark pixel is the repair grey, premultiplied by a=1.
				if r>>8 != 0x77 || g>>8 != 0x77 || bl>>8 != 0x77 {
					t.Fatalf("opaque mark pixel is %v,%v,%v -- want #777777", r>>8, g>>8, bl>>8)
				}
			} else {
				partial++
			}
		}
	}
	if opaque == 0 {
		t.Fatal("the mark disappeared entirely")
	}
	// Anti-aliased edges must survive as partial coverage, not a hard fringe.
	if partial == 0 {
		t.Fatal("no partially transparent pixels -- the repair hard-keyed instead of using coverage")
	}
}

// ---- I4d ------------------------------------------------------------------------------

func TestRepairNoopForPhoto(t *testing.T) {
	cases := []struct {
		name string
		raw  []byte
		ext  string
	}{
		{"shala_hero2.jpg", fixture(t, "shala_hero2.jpg"), "jpg"},
		{"synthetic studio PNG", encodeSyntheticPNG(t, studioPhoto()), "png"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			img, v, err := ClassifyRaw(c.raw, c.ext)
			if err != nil {
				t.Fatal(err)
			}
			if v.Class != ClassPhoto {
				t.Fatalf("classified %q, want %q", v.Class, ClassPhoto)
			}
			if Repairable(v.Class) {
				t.Fatal("a photo must never be repairable")
			}

			out, changed := Repair(img, v)
			if changed {
				t.Fatal("Repair reported a change on a photo")
			}
			if !pixelsEqual(img, out) {
				t.Fatal("Repair altered a photo's pixels")
			}
		})
	}

	// A white-background logo is warn-only for the same reason: keying it would leave dark
	// coloured ink on transparency, the very shape this spec eliminates.
	img, v, err := ClassifyRaw(encodeSyntheticPNG(t, colouredLogoOnWhite()), "png")
	if err != nil {
		t.Fatal(err)
	}
	out, changed := Repair(img, v)
	if changed || !pixelsEqual(img, out) {
		t.Fatal("a white-background logo was repaired; it must only warn")
	}
}

// Guard: the fixtures really are the live files, not placeholders.
func TestFixturesDecode(t *testing.T) {
	for _, f := range []string{
		"social-curated-email.png", "shala_hero.png", "exclusively_on_curated_logo.png",
		"curated_logo3.png", "curated_logo3_grey.png", "shala_hero2.jpg", "social-curated-facebook.png",
	} {
		raw := fixture(t, f)
		if len(raw) < 512 {
			t.Fatalf("%s is %d bytes -- looks like a placeholder", f, len(raw))
		}
		if _, err := DecodeStill(raw); err != nil {
			t.Fatalf("%s does not decode: %v", f, err)
		}
	}
	if _, err := jpeg.Decode(bytes.NewReader(fixture(t, "shala_hero2.jpg"))); err != nil {
		t.Fatalf("the jpg fixture is not a JPEG: %v", err)
	}
}
