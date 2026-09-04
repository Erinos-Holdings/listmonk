package subimporter

// Fork (import presets). An import preset is DATA (settings key app.import_presets, a JSON
// array) that drives a server-side CSV transform and a fill-merge import over the stock
// importer session. A preset declares a header mapping, a locale to lang rule, a filename to
// list-name rule, fixed import options and fill semantics for existing subscribers. Nothing
// in this file knows what any particular preset is for. The functions here are pure where
// they can be (Transform, ListName, LangFor, PlaceholderName) so they can be table-tested
// without a database; the DB-touching ones take a small query interface so the DB harness in
// internal/migrations can drive them against a scratch schema.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gofrs/uuid/v5"
	"github.com/knadh/listmonk/models"
	"github.com/lib/pq"
	"golang.org/x/text/cases"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/language"
)

const (
	// MergeFill is the only merge mode a preset may declare. Existing subscribers keep a
	// real name, keep every attribs key the file does not carry, and get lang only when
	// theirs is blank.
	MergeFill = "fill"

	dedupeNameLongestFirst  = "longest-first"
	dedupeLocaleFirstMapped = "first-mapped"
	dedupeAttribsLast       = "last"

	// EncodingUTF8 and EncodingWindows1252 are the two encodings DecodeText reports.
	EncodingUTF8        = "utf-8"
	EncodingWindows1252 = "windows-1252"

	presetColEmail  = "email"
	presetColName   = "name"
	presetColLocale = "locale"
)

var (
	// ErrHashMismatch is returned when the content_hash presented with an import does not
	// match the bytes received, i.e. the file being imported is not the one previewed.
	ErrHashMismatch = errors.New("content hash does not match the uploaded file")
	// ErrNoValidRows is returned when a file yields zero importable subscribers.
	ErrNoValidRows = errors.New("the file has no valid rows")
	// ErrListAmbiguous is returned when more than one list carries the resolved name.
	ErrListAmbiguous = errors.New("more than one list has the resolved name")
	// ErrEmailColumnMissing is returned when the header lacks the preset's email column.
	ErrEmailColumnMissing = errors.New("email column not found in the header")

	regexPresetKey   = regexp.MustCompile(`^[a-z0-9_-]+$`)
	regexPlaceholder = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)
	regexSpaces      = regexp.MustCompile(`\s+`)
	utf8BOM          = []byte{0xEF, 0xBB, 0xBF}
)

// FilenameError is returned by ListName when the filename does not match the preset's
// pattern. It carries the pattern so the caller can show it.
type FilenameError struct {
	Filename string
	Pattern  string
}

func (e *FilenameError) Error() string {
	return fmt.Sprintf("filename %q does not match the preset pattern %s", e.Filename, e.Pattern)
}

// PresetDedupe declares how repeated emails within one file are merged.
type PresetDedupe struct {
	Name    string `json:"name"`
	Locale  string `json:"locale"`
	Attribs string `json:"attribs"`
}

// Preset is one entry of the app.import_presets settings array.
type Preset struct {
	Key                string            `json:"key"`
	Name               string            `json:"name"`
	Columns            map[string]string `json:"columns"`
	AttribColumns      map[string]string `json:"attrib_columns"`
	ListNamePattern    string            `json:"list_name_pattern"`
	ListNameTemplate   string            `json:"list_name_template"`
	ListType           string            `json:"list_type"`
	ListOptin          string            `json:"list_optin"`
	SubscriptionStatus string            `json:"subscription_status"`
	Backfill           *bool             `json:"backfill"`
	Merge              string            `json:"merge"`
	SkipEmailPattern   string            `json:"skip_email_pattern"`
	// DisplayHeaders is the column list shown to the user as "the exact column names", in
	// the source file's own order. Optional; when empty it is derived from the mappings.
	DisplayHeaders []string     `json:"headers"`
	Dedupe         PresetDedupe `json:"dedupe"`

	listRe *regexp.Regexp
	skipRe *regexp.Regexp
	langs  map[string]struct{}
}

// PresetInfo is the public shape of a preset (what /api/config exposes).
type PresetInfo struct {
	Key  string `json:"key"`
	Name string `json:"name"`
	// Headers is the exact column-name set the preset reads, in a stable order (email,
	// name, locale roles first, then attribute columns sorted), for the UI's instructions.
	Headers []string `json:"headers"`
}

// ParsePresets parses and validates the app.import_presets JSON. langs is the closed set
// of campaign languages a locale may map to. Any invalid preset fails the whole set; the
// caller is expected to log and run with no presets rather than refuse to boot.
func ParsePresets(b []byte, langs []string) ([]Preset, error) {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || bytes.Equal(b, []byte("null")) {
		return nil, nil
	}

	var out []Preset
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	seen := map[string]struct{}{}
	for i := range out {
		p := &out[i]
		if err := p.validate(langs); err != nil {
			return nil, fmt.Errorf("preset %d (%q): %w", i, p.Key, err)
		}
		if _, dup := seen[p.Key]; dup {
			return nil, fmt.Errorf("preset %d: duplicate key %q", i, p.Key)
		}
		seen[p.Key] = struct{}{}
	}

	return out, nil
}

func (p *Preset) validate(langs []string) error {
	if !regexPresetKey.MatchString(p.Key) {
		return errors.New("key must be non-empty [a-z0-9_-]")
	}
	if strings.TrimSpace(p.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(p.Columns[presetColEmail]) == "" {
		return errors.New("columns.email is required")
	}
	for k := range p.Columns {
		switch k {
		case presetColEmail, presetColName, presetColLocale:
		default:
			return fmt.Errorf("unknown column role %q", k)
		}
	}
	// headers, when given, must name exactly the mapped columns (each once) -- it is a
	// display order, never a second source of truth for what is read.
	if len(p.DisplayHeaders) > 0 {
		known := map[string]bool{}
		for _, h := range p.Columns {
			known[h] = true
		}
		for h := range p.AttribColumns {
			known[h] = true
		}
		seen := map[string]bool{}
		for _, h := range p.DisplayHeaders {
			if !known[h] || seen[h] {
				return fmt.Errorf("headers entry %q is not a mapped column or repeats", h)
			}
			seen[h] = true
		}
		if len(seen) != len(known) {
			return errors.New("headers must list every mapped column")
		}
	}
	for hdr, key := range p.AttribColumns {
		if strings.TrimSpace(hdr) == "" || strings.TrimSpace(key) == "" {
			return errors.New("attrib_columns entries must map a header to an attribute key")
		}
		if key == "lang" {
			return errors.New("attrib_columns may not target lang (it is derived from the locale column)")
		}
	}

	re, err := regexp.Compile(p.ListNamePattern)
	if err != nil {
		return fmt.Errorf("list_name_pattern: %w", err)
	}
	p.listRe = re
	groups := map[string]struct{}{}
	for _, g := range re.SubexpNames() {
		if g != "" {
			groups[g] = struct{}{}
		}
	}
	if strings.TrimSpace(p.ListNameTemplate) == "" {
		return errors.New("list_name_template is required")
	}
	for _, m := range regexPlaceholder.FindAllStringSubmatch(p.ListNameTemplate, -1) {
		if _, ok := groups[placeholderGroup(m[1])]; !ok {
			return fmt.Errorf("list_name_template placeholder {%s} names no group in list_name_pattern", m[1])
		}
	}

	switch p.ListType {
	case "":
		p.ListType = models.ListTypePrivate
	case models.ListTypePrivate, models.ListTypePublic:
	default:
		return fmt.Errorf("unknown list_type %q", p.ListType)
	}
	switch p.ListOptin {
	case "":
		p.ListOptin = models.ListOptinSingle
	case models.ListOptinSingle, models.ListOptinDouble:
	default:
		return fmt.Errorf("unknown list_optin %q", p.ListOptin)
	}
	switch p.SubscriptionStatus {
	case "":
		p.SubscriptionStatus = models.SubscriptionStatusConfirmed
	case models.SubscriptionStatusConfirmed, models.SubscriptionStatusUnconfirmed, models.SubscriptionStatusUnsubscribed:
	default:
		return fmt.Errorf("unknown subscription_status %q", p.SubscriptionStatus)
	}
	if p.Backfill == nil {
		// Imported people are not new. Absent means true so a preset can never welcome by omission.
		t := true
		p.Backfill = &t
	}
	if p.Merge != MergeFill {
		return fmt.Errorf("unknown merge %q (only %q is supported)", p.Merge, MergeFill)
	}

	if p.SkipEmailPattern != "" {
		re, err := regexp.Compile(p.SkipEmailPattern)
		if err != nil {
			return fmt.Errorf("skip_email_pattern: %w", err)
		}
		p.skipRe = re
	}

	switch p.Dedupe.Name {
	case "":
		p.Dedupe.Name = dedupeNameLongestFirst
	case dedupeNameLongestFirst:
	default:
		return fmt.Errorf("unknown dedupe.name %q", p.Dedupe.Name)
	}
	switch p.Dedupe.Locale {
	case "":
		p.Dedupe.Locale = dedupeLocaleFirstMapped
	case dedupeLocaleFirstMapped:
	default:
		return fmt.Errorf("unknown dedupe.locale %q", p.Dedupe.Locale)
	}
	switch p.Dedupe.Attribs {
	case "":
		p.Dedupe.Attribs = dedupeAttribsLast
	case dedupeAttribsLast:
	default:
		return fmt.Errorf("unknown dedupe.attribs %q", p.Dedupe.Attribs)
	}

	p.langs = make(map[string]struct{}, len(langs))
	for _, l := range langs {
		p.langs[strings.ToLower(l)] = struct{}{}
	}

	return nil
}

// placeholderGroup maps a template placeholder to the pattern group it reads. {kind} reads
// group kind verbatim; {Kind} reads group kind and title-cases it.
func placeholderGroup(ph string) string {
	if ph == "" {
		return ph
	}
	r, size := utf8.DecodeRuneInString(ph)
	if unicode.IsUpper(r) {
		return string(unicode.ToLower(r)) + ph[size:]
	}
	return ph
}

// Info returns the public shape of the preset.
func (p *Preset) Info() PresetInfo {
	return PresetInfo{Key: p.Key, Name: p.Name, Headers: p.Headers()}
}

// Headers returns the CSV column names the preset expects, for the UI's "exact column
// names" line. The preset's own headers list wins (the source file's order); otherwise
// role columns in the fixed order email, name, locale, then attribute columns sorted.
func (p *Preset) Headers() []string {
	if len(p.DisplayHeaders) > 0 {
		return append([]string(nil), p.DisplayHeaders...)
	}
	out := make([]string, 0, len(p.Columns)+len(p.AttribColumns))
	for _, role := range []string{presetColEmail, presetColName, presetColLocale} {
		if h, ok := p.Columns[role]; ok && h != "" {
			out = append(out, h)
		}
	}
	attribHdrs := make([]string, 0, len(p.AttribColumns))
	for h := range p.AttribColumns {
		attribHdrs = append(attribHdrs, h)
	}
	sort.Strings(attribHdrs)
	return append(out, attribHdrs...)
}

// SessionOpt builds the importer options for this preset. Every option is fixed by the
// preset; nothing about the session is client-controlled beyond the file itself.
func (p *Preset) SessionOpt(filename string, listID int) SessionOpt {
	return SessionOpt{
		Filename:           filename,
		Mode:               ModeSubscribe,
		SubStatus:          p.SubscriptionStatus,
		Overwrite:          false,
		OverwriteUserInfo:  false,
		OverwriteSubStatus: false,
		Backfill:           *p.Backfill,
		Merge:              p.Merge,
		Delim:              ",",
		ListIDs:            []int{listID},
	}
}

// ListName resolves the target list name from the uploaded filename per the preset's
// pattern and template. A non-matching filename is a *FilenameError.
func (p *Preset) ListName(filename string) (string, error) {
	m := p.listRe.FindStringSubmatch(filename)
	if m == nil {
		return "", &FilenameError{Filename: filename, Pattern: p.ListNamePattern}
	}
	groups := map[string]string{}
	for i, g := range p.listRe.SubexpNames() {
		if g != "" && i < len(m) {
			groups[g] = m[i]
		}
	}
	out := regexPlaceholder.ReplaceAllStringFunc(p.ListNameTemplate, func(ph string) string {
		name := ph[1 : len(ph)-1]
		v := groups[placeholderGroup(name)]
		if placeholderGroup(name) != name {
			v = cases.Title(language.Und).String(v)
		}
		return v
	})
	return strings.TrimSpace(out), nil
}

// LangFor derives the campaign language from a locale. The primary subtag (split on - or _)
// is lowercased and kept only when it is one of the preset's allowed languages; anything
// else yields "" (no lang key).
func (p *Preset) LangFor(locale string) string {
	locale = strings.TrimSpace(strings.ToLower(locale))
	if locale == "" {
		return ""
	}
	primary := strings.FieldsFunc(locale, func(r rune) bool { return r == '-' || r == '_' })
	if len(primary) == 0 {
		return ""
	}
	if _, ok := p.langs[primary[0]]; ok {
		return primary[0]
	}
	return ""
}

// PlaceholderName is the name listmonk derives for a subscriber whose CSV/API name is
// empty. The local part of the email, dots to spaces, each word title-cased. This is the
// single definition shared by ValidateFields and the fill upsert so the two cannot drift.
func PlaceholderName(email string) string {
	name := strings.ToLower(strings.Split(email, "@")[0])
	parts := strings.Fields(strings.ReplaceAll(name, ".", " "))
	for n, p := range parts {
		parts[n] = cases.Title(language.Und).String(p)
	}
	return strings.Join(parts, " ")
}

// DecodeText turns uploaded bytes into text. A UTF-8 BOM is stripped. Valid UTF-8 is used
// as-is; anything else is decoded as Windows-1252. The detected encoding is returned so a
// mis-detected file can be shown before it is committed.
func DecodeText(b []byte) (string, string) {
	b = bytes.TrimPrefix(b, utf8BOM)
	if utf8.Valid(b) {
		return string(b), EncodingUTF8
	}
	out, err := charmap.Windows1252.NewDecoder().Bytes(b)
	if err != nil {
		// The 1252 decoder substitutes rather than fails; keep the raw bytes as a last resort.
		return string(b), EncodingUTF8
	}
	return string(out), EncodingWindows1252
}

// ContentHash is the hex SHA-256 of the upload, the token that ties a confirm to its preview.
func ContentHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Skipped is a row the transform did not import.
type Skipped struct {
	Row    int    `json:"row"`
	Email  string `json:"email"`
	Reason string `json:"reason"`
}

// Duplicate is an email that appears on more than one row.
type Duplicate struct {
	Email string `json:"email"`
	Rows  []int  `json:"rows"`
}

// UnmappedLocale is a subscriber whose locale maps to no supported language.
type UnmappedLocale struct {
	Email  string `json:"email"`
	Locale string `json:"locale"`
}

// Parsed is the outcome of Transform: the normalised subscribers plus everything the
// preview reports about the file itself.
type Parsed struct {
	Encoding      string           `json:"encoding"`
	NonASCIINames []string         `json:"non_ascii_names"`
	Rows          int              `json:"rows"`
	Subs          []SubReq         `json:"-"`
	Duplicates    []Duplicate      `json:"duplicates"`
	Unmapped      []UnmappedLocale `json:"unmapped_locales"`
	LangLess      int              `json:"lang_less"`
	Skipped       []Skipped        `json:"skipped"`
	Warnings      []string         `json:"warnings"`
}

// mergedRow is the per-email accumulator used while deduping.
type mergedRow struct {
	email   string
	name    string
	locale  string
	lang    string
	attribs map[string]string
	rows    []int
}

// Transform decodes, parses, validates, skips and dedupes the file into normalised
// subscriber rows. It touches no database. im supplies the stock email validation
// (domain allow/blocklists included).
func (p *Preset) Transform(im *Importer, data []byte) (*Parsed, error) {
	text, enc := DecodeText(data)
	out := &Parsed{
		Encoding:      enc,
		NonASCIINames: []string{},
		Duplicates:    []Duplicate{},
		Unmapped:      []UnmappedLocale{},
		Skipped:       []Skipped{},
		Warnings:      []string{},
	}

	rd := csv.NewReader(strings.NewReader(text))
	rd.Comma = ','
	rd.LazyQuotes = true
	rd.FieldsPerRecord = -1
	rd.TrimLeadingSpace = true

	hdr, err := rd.Read()
	if err != nil {
		return nil, fmt.Errorf("error reading header: %w", err)
	}
	idx := map[string]int{}
	for i, h := range hdr {
		h = strings.ToLower(strings.TrimSpace(h))
		if h == "" {
			continue
		}
		if _, dup := idx[h]; !dup {
			idx[h] = i
		}
	}
	col := func(role string) (int, bool) {
		h, ok := p.Columns[role]
		if !ok || strings.TrimSpace(h) == "" {
			return -1, false
		}
		i, ok := idx[strings.ToLower(strings.TrimSpace(h))]
		return i, ok
	}
	emailIdx, ok := col(presetColEmail)
	if !ok {
		return nil, fmt.Errorf("%w (%q)", ErrEmailColumnMissing, p.Columns[presetColEmail])
	}
	nameIdx, hasName := col(presetColName)
	if _, declared := p.Columns[presetColName]; declared && !hasName {
		out.Warnings = append(out.Warnings, fmt.Sprintf("name column %q not found; names will be derived from emails", p.Columns[presetColName]))
	}
	localeIdx, hasLocale := col(presetColLocale)
	if _, declared := p.Columns[presetColLocale]; declared && !hasLocale {
		out.Warnings = append(out.Warnings, fmt.Sprintf("locale column %q not found; no language will be set", p.Columns[presetColLocale]))
	}
	type attribCol struct {
		key string
		idx int
	}
	attribCols := []attribCol{}
	attribHdrs := make([]string, 0, len(p.AttribColumns))
	for h := range p.AttribColumns {
		attribHdrs = append(attribHdrs, h)
	}
	sort.Strings(attribHdrs)
	for _, h := range attribHdrs {
		i, ok := idx[strings.ToLower(strings.TrimSpace(h))]
		if !ok {
			out.Warnings = append(out.Warnings, fmt.Sprintf("attribute column %q not found; %q will not be set", h, p.AttribColumns[h]))
			continue
		}
		attribCols = append(attribCols, attribCol{key: p.AttribColumns[h], idx: i})
	}

	cell := func(cols []string, i int) string {
		if i < 0 || i >= len(cols) {
			return ""
		}
		return strings.TrimSpace(cols[i])
	}

	var (
		merged  = map[string]*mergedRow{}
		order   []string
		nonASCI = map[string]struct{}{}
		line    = 1
	)
	for {
		line++
		cols, err := rd.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading CSV line %d: %w", line, err)
		}
		// A fully blank line is not a row.
		blank := true
		for _, c := range cols {
			if strings.TrimSpace(c) != "" {
				blank = false
				break
			}
		}
		if blank {
			continue
		}
		out.Rows++

		rawEmail := cell(cols, emailIdx)
		email, err := im.ValidateEmail(rawEmail)
		if err != nil {
			out.Skipped = append(out.Skipped, Skipped{Row: line, Email: rawEmail, Reason: err.Error()})
			continue
		}
		if p.skipRe != nil && p.skipRe.MatchString(email) {
			out.Skipped = append(out.Skipped, Skipped{Row: line, Email: email, Reason: "matches skip pattern " + p.SkipEmailPattern})
			continue
		}

		name := ""
		if hasName {
			name = strings.TrimSpace(regexSpaces.ReplaceAllString(cell(cols, nameIdx), " "))
		}
		if name != "" && !isASCII(name) {
			nonASCI[name] = struct{}{}
		}
		locale := ""
		if hasLocale {
			locale = cell(cols, localeIdx)
		}
		attribs := map[string]string{}
		for _, ac := range attribCols {
			if v := cell(cols, ac.idx); v != "" {
				attribs[ac.key] = v
			}
		}

		m, exists := merged[email]
		if !exists {
			merged[email] = &mergedRow{
				email: email, name: name, locale: locale, lang: p.LangFor(locale),
				attribs: attribs, rows: []int{line},
			}
			order = append(order, email)
			continue
		}

		// Dedupe. Name = longest trimmed non-empty, ties keep the first row. Locale = the
		// first that maps to a supported lang. Attribs = last row's non-empty value per key.
		m.rows = append(m.rows, line)
		if len([]rune(name)) > len([]rune(m.name)) {
			m.name = name
		}
		if m.lang == "" {
			if l := p.LangFor(locale); l != "" {
				m.locale, m.lang = locale, l
			} else if m.locale == "" {
				m.locale = locale
			}
		}
		for k, v := range attribs {
			m.attribs[k] = v
		}
	}

	for _, email := range order {
		m := merged[email]
		if len(m.rows) > 1 {
			out.Duplicates = append(out.Duplicates, Duplicate{Email: email, Rows: m.rows})
		}
		if m.lang == "" {
			out.LangLess++
			if m.locale != "" {
				out.Unmapped = append(out.Unmapped, UnmappedLocale{Email: email, Locale: m.locale})
			}
		}

		sub := SubReq{}
		sub.Email = email
		sub.Name = m.name
		// An empty name yields the placeholder, which the fill upsert treats as no information.
		sub, err := im.ValidateFields(sub)
		if err != nil {
			out.Skipped = append(out.Skipped, Skipped{Row: m.rows[0], Email: email, Reason: err.Error()})
			continue
		}
		sub.Attribs = models.JSON{}
		if m.lang != "" {
			sub.Attribs["lang"] = m.lang
		}
		for k, v := range m.attribs {
			sub.Attribs[k] = v
		}
		sub.Backfill = *p.Backfill
		out.Subs = append(out.Subs, sub)
	}

	for n := range nonASCI {
		out.NonASCIINames = append(out.NonASCIINames, n)
	}
	sort.Strings(out.NonASCIINames)

	return out, nil
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// ValidateEmail sanitizes and validates a bare email the way ValidateFields does, without
// touching the name.
func (im *Importer) ValidateEmail(email string) (string, error) {
	if len(email) > 1000 {
		return "", errors.New(im.i18n.T("subscribers.invalidEmail"))
	}
	em, err := im.SanitizeEmail(email)
	if err != nil {
		return "", err
	}
	return strings.ToLower(em), nil
}

// Querier is the slice of *sql.DB the preset DB functions need.
type Querier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// ListInfo describes the resolved target list.
type ListInfo struct {
	Name            string `json:"name"`
	ID              int    `json:"id,omitempty"`
	Exists          bool   `json:"exists"`
	SubscriberCount int    `json:"subscriber_count,omitempty"`
}

// FillName is an existing subscriber whose name the import will set.
type FillName struct {
	Email string `json:"email"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// FillLang is an existing subscriber whose lang the import will set.
type FillLang struct {
	Email string `json:"email"`
	Lang  string `json:"lang"`
}

// PreviewResult is the preview JSON.
type PreviewResult struct {
	ContentHash string   `json:"content_hash"`
	List        ListInfo `json:"list"`
	*Parsed
	Subscribers  int        `json:"subscribers"`
	New          int        `json:"new"`
	Existing     int        `json:"existing"`
	WillFillName []FillName `json:"will_fill_name"`
	WillFillLang []FillLang `json:"will_fill_lang"`
}

// ResolveList finds the list of exactly the given name. Zero matches is a list to create,
// one is the list, more than one is ErrListAmbiguous.
func ResolveList(ctx context.Context, db Querier, name string) (ListInfo, error) {
	rows, err := db.QueryContext(ctx, `SELECT id FROM lists WHERE name = $1 ORDER BY id`, name)
	if err != nil {
		return ListInfo{}, err
	}
	defer rows.Close()
	ids := []int{}
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return ListInfo{}, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return ListInfo{}, err
	}

	info := ListInfo{Name: name}
	switch len(ids) {
	case 0:
		return info, nil
	case 1:
		info.ID, info.Exists = ids[0], true
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriber_lists WHERE list_id = $1 AND status <> 'unsubscribed'`, info.ID).Scan(&info.SubscriberCount); err != nil {
			return ListInfo{}, err
		}
		return info, nil
	default:
		return info, fmt.Errorf("%w: %q (ids %v)", ErrListAmbiguous, name, ids)
	}
}

// existingSub is what the preview reads about a subscriber already in the DB.
type existingSub struct {
	name string
	lang string
}

// existingByEmail looks the given (lowercased) emails up by LOWER(email) in one query.
func existingByEmail(ctx context.Context, db Querier, emails []string) (map[string]existingSub, error) {
	out := map[string]existingSub{}
	if len(emails) == 0 {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT LOWER(email), name, COALESCE(attribs->>'lang', '') FROM subscribers WHERE LOWER(email) = ANY($1)`, pq.Array(emails))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var email string
		var s existingSub
		if err := rows.Scan(&email, &s.name, &s.lang); err != nil {
			return nil, err
		}
		out[email] = s
	}
	return out, rows.Err()
}

// Preview parses the file and reports what an import would do. It writes nothing and
// opens no importer session.
func Preview(ctx context.Context, db Querier, im *Importer, p *Preset, filename string, data []byte) (*PreviewResult, error) {
	listName, err := p.ListName(filename)
	if err != nil {
		return nil, err
	}
	parsed, err := p.Transform(im, data)
	if err != nil {
		return nil, err
	}
	list, err := ResolveList(ctx, db, listName)
	if err != nil {
		return nil, err
	}

	emails := make([]string, 0, len(parsed.Subs))
	for _, s := range parsed.Subs {
		emails = append(emails, s.Email)
	}
	existing, err := existingByEmail(ctx, db, emails)
	if err != nil {
		return nil, err
	}

	res := &PreviewResult{
		ContentHash:  ContentHash(data),
		List:         list,
		Parsed:       parsed,
		Subscribers:  len(parsed.Subs),
		WillFillName: []FillName{},
		WillFillLang: []FillLang{},
	}
	for _, s := range parsed.Subs {
		ex, ok := existing[s.Email]
		if !ok {
			res.New++
			continue
		}
		res.Existing++
		if (ex.name == "" || ex.name == PlaceholderName(s.Email)) && s.Name != ex.name {
			res.WillFillName = append(res.WillFillName, FillName{Email: s.Email, From: ex.name, To: s.Name})
		}
		if l, ok := s.Attribs["lang"].(string); ok && l != "" && strings.TrimSpace(ex.lang) == "" {
			res.WillFillLang = append(res.WillFillLang, FillLang{Email: s.Email, Lang: l})
		}
	}

	return res, nil
}

// Prepared is a validated, list-resolved import ready to run.
type Prepared struct {
	List   ListInfo
	Parsed *Parsed
}

// PrepareImport re-parses the upload for the confirm step. The content hash must match the
// bytes received, the file must yield at least one valid row (checked BEFORE any list is
// created), and the target list is created when absent. It does not start a session.
func PrepareImport(ctx context.Context, db Querier, im *Importer, p *Preset, filename string, data []byte, contentHash string) (*Prepared, error) {
	if !strings.EqualFold(strings.TrimSpace(contentHash), ContentHash(data)) {
		return nil, ErrHashMismatch
	}
	listName, err := p.ListName(filename)
	if err != nil {
		return nil, err
	}
	parsed, err := p.Transform(im, data)
	if err != nil {
		return nil, err
	}
	if len(parsed.Subs) == 0 {
		return nil, ErrNoValidRows
	}
	list, err := ResolveList(ctx, db, listName)
	if err != nil {
		return nil, err
	}
	if !list.Exists {
		uu, err := uuid.NewV4()
		if err != nil {
			return nil, err
		}
		if err := db.QueryRowContext(ctx,
			`INSERT INTO lists (uuid, name, type, optin, status, tags, description) VALUES ($1, $2, $3, $4, $5, '{}', '') RETURNING id`,
			uu.String(), listName, p.ListType, p.ListOptin, models.ListStatusActive).Scan(&list.ID); err != nil {
			return nil, fmt.Errorf("error creating list %q: %w", listName, err)
		}
	}

	return &Prepared{List: list, Parsed: parsed}, nil
}

// RunPreset opens a session with the preset's fixed options and feeds it the normalised
// rows. Both halves run as goroutines, like the stock CSV path.
func (im *Importer) RunPreset(p *Preset, filename string, listID int, subs []SubReq) error {
	sess, err := im.NewSession(p.SessionOpt(filename, listID))
	if err != nil {
		return err
	}
	go sess.Start()
	go sess.LoadRows(subs)
	return nil
}
