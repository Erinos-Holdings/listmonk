package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/knadh/listmonk/internal/media"
	"github.com/knadh/listmonk/internal/media/optimizer"
	"github.com/knadh/listmonk/models"
)

// Originals retention and reprocessing (DARK-MODE-SPEC D4/D5, invariants I4c / I5a / I5b).
//
// The store side is exercised against a fake media.Store that records every call, so these
// run in plain `go test ./cmd/...` with no database: the DB read and the meta write live in
// the handlers, everything that touches objects lives in the functions tested here.

type fakeStore struct {
	objects map[string][]byte
	puts    []string
	deletes []string
	getErr  map[string]error
}

func newFakeStore() *fakeStore {
	return &fakeStore{objects: map[string][]byte{}, getErr: map[string]error{}}
}

func (f *fakeStore) Put(name, _ string, r io.ReadSeeker) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	f.objects[name] = b
	f.puts = append(f.puts, name)
	return name, nil
}

func (f *fakeStore) Delete(name string) error {
	f.deletes = append(f.deletes, name)
	delete(f.objects, name)
	return nil
}

func (f *fakeStore) GetURL(name string) string { return "https://example.test/uploads/" + name }

func (f *fakeStore) GetBlob(name string) ([]byte, error) {
	if err, ok := f.getErr[name]; ok {
		return nil, err
	}
	b, ok := f.objects[name]
	if !ok {
		return nil, errors.New("not found")
	}
	return b, nil
}

var _ media.Store = (*fakeStore)(nil)

func testFixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "internal", "media", "optimizer", "testdata", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return raw
}

// ---- I4c: originals retained, and deleted with the row -------------------------------

func TestPutOriginalNeverOverwrites(t *testing.T) {
	s := newFakeStore()

	first := []byte("the true original")
	if _, err := putOriginal(s, "logo.png", "image/png", first); err != nil {
		t.Fatal(err)
	}
	if got := string(s.objects["orig_logo.png"]); got != string(first) {
		t.Fatalf("original is %q", got)
	}

	// H4: a second call -- a reprocess -- must NOT replace it. Overwriting would make the
	// repair irreversible and make the next reprocess repair already-repaired bytes.
	if _, err := putOriginal(s, "logo.png", "image/png", []byte("repaired bytes")); err != nil {
		t.Fatal(err)
	}
	if got := string(s.objects["orig_logo.png"]); got != string(first) {
		t.Fatalf("the original was overwritten: %q", got)
	}
	if len(s.puts) != 1 {
		t.Fatalf("the second call wrote: %v", s.puts)
	}
}

func TestOriginalBytesPrefersTheOriginal(t *testing.T) {
	s := newFakeStore()
	s.objects["logo.png"] = []byte("repaired")
	s.objects["orig_logo.png"] = []byte("original")

	b, err := originalBytes(s, "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "original" {
		t.Fatalf("read %q, want the original", b)
	}

	// With no original kept, the stored object is the source.
	delete(s.objects, "orig_logo.png")
	if b, err = originalBytes(s, "logo.png"); err != nil || string(b) != "repaired" {
		t.Fatalf("read %q, %v", b, err)
	}
}

func TestDeleteRemovesAllThreeObjects(t *testing.T) {
	s := newFakeStore()
	s.objects["logo.png"] = []byte("x")
	s.objects["thumb_logo.png"] = []byte("x")
	s.objects["orig_logo.png"] = []byte("x")

	deleteMediaObjects(s, "logo.png")

	for _, k := range []string{"logo.png", "thumb_logo.png", "orig_logo.png"} {
		if !contains(s.deletes, k) {
			t.Fatalf("%s was not deleted (deletes: %v)", k, s.deletes)
		}
	}
}

func TestUploadClassificationStoresAnOriginalOnlyWhenPixelsChange(t *testing.T) {
	cases := []struct {
		file      string
		wantFixed bool
	}{
		{"curated_logo3.png", true},            // mono-on-white -> keyed to #777 on alpha
		{"social-curated-email.png", true},     // icon-ring -> white disc filled in
		{"social-curated-facebook.png", false}, // already fine
		{"shala_hero.png", true},               // mono-on-transparent -> recoloured to #777, alpha kept
		{"shala_hero2.jpg", false},             // photo
	}

	for _, c := range cases {
		t.Run(c.file, func(t *testing.T) {
			raw := testFixture(t, c.file)
			ext := "png"
			if filepath.Ext(c.file) == ".jpg" {
				ext = "jpg"
			}

			out, outExt, v, fixed, err := classifyAndRepair(raw, ext)
			if err != nil {
				t.Fatal(err)
			}
			if fixed != c.wantFixed {
				t.Fatalf("%s: fixed=%v (class %q), want %v", c.file, fixed, v.Class, c.wantFixed)
			}

			s := newFakeStore()
			if fixed {
				if outExt != "png" {
					t.Fatalf("a repair must encode PNG (alpha), got %q", outExt)
				}
				if bytes.Equal(out, raw) {
					t.Fatal("fixed=true but the bytes are identical")
				}
				if _, err := putOriginal(s, c.file, "image/png", raw); err != nil {
					t.Fatal(err)
				}
			}

			// I4c: an altering upload stores orig_<name>; a non-altering one stores
			// nothing extra.
			_, hasOrig := s.objects["orig_"+c.file]
			if hasOrig != c.wantFixed {
				t.Fatalf("orig_ present=%v, want %v", hasOrig, c.wantFixed)
			}

			meta := models.JSON{darkmodeMetaKey: verdictMeta(v, fixed)}
			if fixed {
				meta[originalMetaKey] = true
			}
			if _, ok := meta[originalMetaKey]; ok != c.wantFixed {
				t.Fatalf("meta.original present=%v, want %v", ok, c.wantFixed)
			}

			d, ok := meta[darkmodeMetaKey].(models.JSON)
			if !ok {
				t.Fatalf("meta.darkmode is %T", meta[darkmodeMetaKey])
			}
			if d["class"] != v.Class || d["fixed"] != fixed || d["version"] != optimizer.ClassifierVersion {
				t.Fatalf("meta.darkmode = %v", d)
			}
			if d["at"] == "" {
				t.Fatal("meta.darkmode carries no timestamp")
			}
		})
	}
}

// ---- I5a: the GET verdict path writes nothing ----------------------------------------

func TestClassifyPathWritesNothing(t *testing.T) {
	s := newFakeStore()
	s.objects["curated_logo3.png"] = testFixture(t, "curated_logo3.png")

	// This is exactly what GetMediaDarkmode does to the store: one read, nothing else.
	raw, err := originalBytes(s, "curated_logo3.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, v, err := optimizer.ClassifyRaw(raw, "png"); err != nil || v.Class != optimizer.ClassMonoOnWhite {
		t.Fatalf("classify: %q %v", v.Class, err)
	}

	if len(s.puts) != 0 {
		t.Fatalf("the verdict path wrote objects: %v", s.puts)
	}
	if len(s.deletes) != 0 {
		t.Fatalf("the verdict path deleted objects: %v", s.deletes)
	}
}

// ---- I5b: reprocess repairs from the original and never overwrites it ------------------

func TestReprocessRepairsFromTheOriginal(t *testing.T) {
	original := testFixture(t, "curated_logo3.png")

	s := newFakeStore()
	s.objects["curated_logo3.png"] = original
	row := mediaLike{ID: 7, Filename: "curated_logo3.png", ContentType: "image/png"}

	meta, out, err := reprocessStored(s, row, false)
	if err != nil {
		t.Fatal(err)
	}
	if out["fixed"] != true || out["class"] != optimizer.ClassMonoOnWhite {
		t.Fatalf("first pass: %v", out)
	}
	if meta[originalMetaKey] != true {
		t.Fatalf("meta did not record the retained original: %v", meta)
	}
	if !contains(s.puts, "orig_curated_logo3.png") {
		t.Fatalf("no original was stored: %v", s.puts)
	}
	if !contains(s.puts, "curated_logo3.png") || !contains(s.puts, "thumb_curated_logo3.png") {
		t.Fatalf("object or thumbnail not rewritten: %v", s.puts)
	}
	if !bytes.Equal(s.objects["orig_curated_logo3.png"], original) {
		t.Fatal("the stored original is not the untouched upload")
	}

	repaired := append([]byte(nil), s.objects["curated_logo3.png"]...)

	// A second reprocess with force: it must read the ORIGINAL again, so it produces the
	// same repaired bytes rather than repairing the repair -- and must not touch orig_.
	row.Meta = models.JSON{darkmodeMetaKey: map[string]any{"version": optimizer.ClassifierVersion}}
	s.puts = nil
	if _, out2, err := reprocessStored(s, row, true); err != nil {
		t.Fatal(err)
	} else if out2["fixed"] != true {
		t.Fatalf("forced pass: %v", out2)
	}

	if !bytes.Equal(s.objects["orig_curated_logo3.png"], original) {
		t.Fatal("the original's bytes changed on the second pass")
	}
	if !bytes.Equal(s.objects["curated_logo3.png"], repaired) {
		t.Fatal("the second pass produced different repaired bytes (it repaired the repair)")
	}
}

func TestReprocessRefusesACurrentVerdictWithoutForce(t *testing.T) {
	s := newFakeStore()
	s.objects["curated_logo3.png"] = testFixture(t, "curated_logo3.png")

	row := mediaLike{
		ID: 7, Filename: "curated_logo3.png", ContentType: "image/png",
		Meta: models.JSON{darkmodeMetaKey: map[string]any{"version": float64(optimizer.ClassifierVersion)}},
	}

	if _, _, err := reprocessStored(s, row, false); !errors.Is(err, ErrVerdictCurrent) {
		t.Fatalf("err = %v, want ErrVerdictCurrent", err)
	}
	if len(s.puts) != 0 {
		t.Fatalf("a refused reprocess wrote: %v", s.puts)
	}

	// A stale verdict is reprocessed without force.
	row.Meta = models.JSON{darkmodeMetaKey: map[string]any{"version": float64(optimizer.ClassifierVersion - 1)}}
	if _, _, err := reprocessStored(s, row, false); err != nil {
		t.Fatal(err)
	}
	if len(s.puts) == 0 {
		t.Fatal("a stale verdict was not reprocessed")
	}
}

func TestReprocessLeavesUnrepairableClassesAlone(t *testing.T) {
	for _, f := range []string{"shala_hero2.jpg", "social-curated-facebook.png"} {
		t.Run(f, func(t *testing.T) {
			s := newFakeStore()
			s.objects[f] = testFixture(t, f)
			before := append([]byte(nil), s.objects[f]...)

			meta, out, err := reprocessStored(s, mediaLike{ID: 1, Filename: f, ContentType: "image/png"}, false)
			if err != nil {
				t.Fatal(err)
			}
			if out["fixed"] != false {
				t.Fatalf("%s was repaired: %v", f, out)
			}
			if _, ok := meta[originalMetaKey]; ok {
				t.Fatal("an unrepaired item recorded a retained original")
			}
			if len(s.puts) != 0 {
				t.Fatalf("an unrepaired item wrote objects: %v", s.puts)
			}
			if !bytes.Equal(s.objects[f], before) {
				t.Fatal("the stored bytes changed")
			}
		})
	}
}

func TestReprocessRefusesNonRaster(t *testing.T) {
	s := newFakeStore()
	s.objects["logo.svg"] = []byte("<svg/>")
	if _, _, err := reprocessStored(s, mediaLike{ID: 1, Filename: "logo.svg"}, false); !errors.Is(err, errNotRaster) {
		t.Fatalf("err = %v, want errNotRaster", err)
	}
	if len(s.puts) != 0 {
		t.Fatalf("wrote: %v", s.puts)
	}
}
