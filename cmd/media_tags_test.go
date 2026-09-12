package main

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/knadh/listmonk/internal/i18n"
	"github.com/labstack/echo/v4"
)

// Fork (media tags, MEDIA-TAGS-SPEC I5, handler half). UploadMedia parses and normalizes the
// `tags` field as its FIRST step, so an invalid tag is a 400 before any DB read or object
// store. The App here is DB-free (no core at all -- any DB read would panic) with the
// dark-mode tests' fakeStore, which records every Put.

func mediaTagsTestApp(t *testing.T) (*App, *fakeStore) {
	t.Helper()
	i, err := i18n.New([]byte(`{"_.code":"en","_.name":"English","media.tagInvalid":"Invalid tag \"{tag}\""}`))
	if err != nil {
		t.Fatal(err)
	}
	s := newFakeStore()
	return &App{i18n: i, media: s}, s
}

func TestUploadMediaRejectsInvalidTagsFirst(t *testing.T) {
	a, s := mediaTagsTestApp(t)

	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	if err := w.WriteField("tags", "shala, Bad Tag"); err != nil {
		t.Fatal(err)
	}
	fw, err := w.CreateFormFile("file", "logo.png")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write(testFixture(t, "curated_logo3.png"))
	w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/media", &body)
	req.Header.Set(echo.HeaderContentType, w.FormDataContentType())
	c := echo.New().NewContext(req, httptest.NewRecorder())

	err = a.UploadMedia(c)

	var he *echo.HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("want *echo.HTTPError, got %v", err)
	}
	if he.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", he.Code)
	}
	if msg, _ := he.Message.(string); !strings.Contains(msg, `"bad tag"`) {
		t.Fatalf("message %q does not name the (folded) bad tag", he.Message)
	}
	if len(s.puts) != 0 {
		t.Fatalf("objects stored before the tag check: %v", s.puts)
	}
}

func TestMediaTagsField(t *testing.T) {
	a, _ := mediaTagsTestApp(t)

	cases := []struct {
		field string
		want  []string
	}{
		{"", []string{}},
		{"  ", []string{}},
		{"Shala, curated,,shala", []string{"curated", "shala"}},
		{"loyalty-rewards", []string{"loyalty-rewards"}},
	}
	for _, c := range cases {
		got, err := a.normalizeMediaTags(splitMediaTagsField(c.field))
		if err != nil {
			t.Fatalf("%q: %v", c.field, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%q: got %v want %v", c.field, got, c.want)
		}
	}

	if _, err := a.normalizeMediaTags([]string{"brand:shala"}); err == nil {
		t.Fatal("brand:shala accepted; want a 400")
	}
}
