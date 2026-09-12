package media

import (
	"io"

	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
	"gopkg.in/volatiletech/null.v6"
)

// Media represents an uploaded object.
type Media struct {
	ID          int         `db:"id" json:"id"`
	UUID        string      `db:"uuid" json:"uuid"`
	Filename    string      `db:"filename" json:"filename"`
	ContentType string      `db:"content_type" json:"content_type"`
	Thumb       string      `db:"thumb" json:"-"`
	CreatedAt   null.Time   `db:"created_at" json:"created_at"`
	ThumbURL    null.String `json:"thumb_url"`
	Provider    string      `json:"provider"`
	Meta        models.JSON `db:"meta" json:"meta"`
	URL         string      `json:"url"`

	// Fork (media tags) -- MEDIA-TAGS-SPEC 3.2. Always serialized as an array, never null
	// (core replaces a nil scan with an empty slice).
	Tags pq.StringArray `db:"tags" json:"tags"`

	Total int `db:"total" json:"-"`
}

// TagCount is one distinct media tag and the number of rows carrying it.
type TagCount struct {
	Tag   string `db:"tag" json:"tag"`
	Count int    `db:"count" json:"count"`
}

// Store represents functions to store and retrieve media (files).
type Store interface {
	Put(string, string, io.ReadSeeker) (string, error)
	Delete(string) error
	GetURL(string) string
	GetBlob(string) ([]byte, error)
}
