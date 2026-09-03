package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/gdgvda/cron"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx/types"
	koanfjson "github.com/knadh/koanf/parsers/json"
	"github.com/knadh/koanf/providers/rawbytes"
	"github.com/knadh/koanf/v2"
	"github.com/knadh/listmonk/internal/auth"
	"github.com/knadh/listmonk/internal/core"
	"github.com/knadh/listmonk/internal/messenger/email"
	"github.com/knadh/listmonk/internal/notifs"
	"github.com/knadh/listmonk/models"
	"github.com/labstack/echo/v4"
)

type aboutHost struct {
	OS       string `json:"os"`
	Machine  string `json:"arch"`
	Hostname string `json:"hostname"`
}

type aboutSystem struct {
	NumCPU  int    `json:"num_cpu"`
	AllocMB uint64 `json:"memory_alloc_mb"`
	OSMB    uint64 `json:"memory_from_os_mb"`
}

type about struct {
	Version   string         `json:"version"`
	Build     string         `json:"build"`
	GoVersion string         `json:"go_version"`
	GoArch    string         `json:"go_arch"`
	Database  types.JSONText `json:"database"`
	System    aboutSystem    `json:"system"`
	Host      aboutHost      `json:"host"`
}

var (
	reAlphaNum = regexp.MustCompile(`[^a-z0-9\-]`)
)

// GetSettings returns settings from the DB.
func (a *App) GetSettings(c echo.Context) error {
	s, err := a.core.GetSettings()
	if err != nil {
		return err
	}

	// Empty out passwords. core.MaskSecret is the display form UpdateSettings refuses to
	// store (fork, 2026-09-03) -- keep the two in the same package so they cannot diverge.
	for i := range s.SMTP {
		s.SMTP[i].Password = core.MaskSecret(s.SMTP[i].Password)
	}
	for i := range s.BounceBoxes {
		s.BounceBoxes[i].Password = core.MaskSecret(s.BounceBoxes[i].Password)
	}
	for i := range s.Messengers {
		s.Messengers[i].Password = core.MaskSecret(s.Messengers[i].Password)
	}

	s.UploadS3AwsSecretAccessKey = core.MaskSecret(s.UploadS3AwsSecretAccessKey)
	s.SendgridKey = core.MaskSecret(s.SendgridKey)
	s.BounceAzure.SharedSecret = core.MaskSecret(s.BounceAzure.SharedSecret)
	s.BouncePostmark.Password = core.MaskSecret(s.BouncePostmark.Password)
	s.BounceForwardEmail.Key = core.MaskSecret(s.BounceForwardEmail.Key)
	s.BounceLettermint.Key = core.MaskSecret(s.BounceLettermint.Key)
	s.SecurityCaptcha.HCaptcha.Secret = core.MaskSecret(s.SecurityCaptcha.HCaptcha.Secret)
	s.OIDC.ClientSecret = core.MaskSecret(s.OIDC.ClientSecret)

	return c.JSON(http.StatusOK, okResp{s})
}

// UpdateSettings returns settings from the DB.
func (a *App) UpdateSettings(c echo.Context) error {
	// Unmarshal and marshal the fields once to sanitize the settings blob.
	var set models.Settings
	if err := c.Bind(&set); err != nil {
		return err
	}

	// Get the existing settings.
	cur, err := a.core.GetSettings()
	if err != nil {
		return err
	}

	// Validate and sanitize postback Messenger names along with SMTP names
	// (where each SMTP is also considered as a standalone messenger).
	// Duplicates are disallowed and "email" is a reserved name.
	names := map[string]bool{emailMsgr: true}

	// There should be at least one SMTP block that's enabled.
	has := false
	for i, s := range set.SMTP {
		if s.Enabled {
			has = true
		}

		// Sanitize and normalize the SMTP server name.
		name := reAlphaNum.ReplaceAllString(strings.ToLower(strings.TrimSpace(s.Name)), "-")
		if name != "" {
			if !strings.HasPrefix(name, "email-") {
				name = "email-" + name
			}

			if _, ok := names[name]; ok {
				return echo.NewHTTPError(http.StatusBadRequest,
					a.i18n.Ts("settings.duplicateMessengerName", "name", name))
			}

			names[name] = true
		}
		set.SMTP[i].Name = name

		// Assign a UUID. The frontend only sends a password when the user explicitly
		// changes the password. In other cases, the existing password in the DB
		// is copied while updating the settings and the UUID is used to match
		// the incoming array of SMTP blocks with the array in the DB.
		if s.UUID == "" {
			set.SMTP[i].UUID = uuid.Must(uuid.NewV4()).String()
		}

		// Ensure the HOST is trimmed of any whitespace.
		// This is a common mistake when copy-pasting SMTP settings.
		set.SMTP[i].Host = strings.TrimSpace(s.Host)

		// Passwords are resolved against the stored ones in resolveSettingsSecrets below.
	}
	if !has {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.errorNoSMTP"))
	}

	// Normalize `from_addresses``. Values are either an e-mail address
	// or an FQDN. Duplicate domains across server blocks are allowed
	// (they get round-robin'd while sending).
	for i, s := range set.SMTP {
		if !s.Enabled {
			continue
		}

		addrs := make([]string, 0, len(s.FromAddresses))
		for _, addr := range s.FromAddresses {
			if k := email.NormalizeAddr(addr); k != "" {
				addrs = append(addrs, k)
			}
		}
		set.SMTP[i].FromAddresses = addrs
	}

	// Always remove the trailing slash from the app root URL.
	set.AppRootURL = strings.TrimRight(set.AppRootURL, "/")

	// Bounce boxes.
	for i, s := range set.BounceBoxes {
		// Assign a UUID. The frontend only sends a password when the user explicitly
		// changes the password. In other cases, the existing password in the DB
		// is copied while updating the settings and the UUID is used to match
		// the incoming array of blocks with the array in the DB.
		if s.UUID == "" {
			set.BounceBoxes[i].UUID = uuid.Must(uuid.NewV4()).String()
		}

		// Ensure the HOST is trimmed of any whitespace.
		// This is a common mistake when copy-pasting SMTP settings.
		set.BounceBoxes[i].Host = strings.TrimSpace(s.Host)

		if d, _ := time.ParseDuration(s.ScanInterval); d.Minutes() < 1 {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.bounces.invalidScanInterval"))
		}

	}

	for i, m := range set.Messengers {
		// UUID to keep track of password changes similar to the SMTP logic above.
		if m.UUID == "" {
			set.Messengers[i].UUID = uuid.Must(uuid.NewV4()).String()
		}

		name := reAlphaNum.ReplaceAllString(strings.ToLower(m.Name), "")
		if _, ok := names[name]; ok {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("settings.duplicateMessengerName", "name", name))
		}
		if len(name) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("settings.invalidMessengerName"))
		}

		set.Messengers[i].Name = name
		names[name] = true
	}

	// Every secret field: empty or display-mask incoming values keep the stored secret; a
	// value merely containing the mask is refused (fork, 2026-09-03).
	if err := resolveSettingsSecrets(&set, cur); err != nil {
		return err
	}

	// OIDC user auto-creation is enabled. Validate.
	if set.OIDC.AutoCreateUsers {
		if set.OIDC.DefaultUserRoleID.Int < auth.SuperAdminRoleID {
			return echo.NewHTTPError(http.StatusBadRequest,
				a.i18n.Ts("globals.messages.invalidFields", "name", a.i18n.T("settings.security.OIDCDefaultRole")))
		}
	}

	for n, v := range set.UploadExtensions {
		set.UploadExtensions[n] = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(v), "."))
	}

	// Domain blocklist / allowlist.
	doms := make([]string, 0, len(set.DomainBlocklist))
	for _, d := range set.DomainBlocklist {
		if d = strings.TrimSpace(strings.ToLower(d)); d != "" {
			doms = append(doms, d)
		}
	}
	set.DomainBlocklist = doms

	doms = make([]string, 0, len(set.DomainAllowlist))
	for _, d := range set.DomainAllowlist {
		if d = strings.TrimSpace(strings.ToLower(d)); d != "" {
			doms = append(doms, d)
		}
	}
	set.DomainAllowlist = doms

	// Validate and clean trusted URLs.
	urls := make([]string, 0, len(set.SecurityTrustedURLs))
	for _, d := range set.SecurityTrustedURLs {
		if d = strings.TrimSpace(d); d != "" {
			if d == "*" {
				urls = append(urls, d)
				continue
			}

			// Parse and validate the URL.
			u, err := url.Parse(d)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return echo.NewHTTPError(http.StatusBadRequest,
					a.i18n.Ts("globals.messages.invalidData")+": invalid trusted URL: "+d)
			}
			urls = append(urls, d)
		}
	}
	set.SecurityTrustedURLs = urls

	// Validate slow query caching cron.
	if set.CacheSlowQueries {
		if _, err := cron.ParseStandard(set.CacheSlowQueriesInterval); err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.invalidData")+": slow query cron: "+err.Error())
		}
	}

	// Update the settings in the DB.
	if err := a.core.UpdateSettings(set); err != nil {
		return err
	}

	return a.handleSettingsRestart(c)
}

// UpdateSettingsByKey updates a single setting key-value in the DB.
func (a *App) UpdateSettingsByKey(c echo.Context) error {
	key := c.Param("key")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
	}

	// Read the raw JSON body as the value.
	var b json.RawMessage
	if err := c.Bind(&b); err != nil {
		return err
	}

	// Fork (2026-09-03): a key that carries secrets goes through the same resolution as the
	// full PUT. Without this, PUT /api/settings/smtp with the array GET returned stores the
	// display mask as the SES password -- the same failure the full-PUT guard closes.
	if secretSettingKeys[key] {
		resolved, err := a.resolveSecretSettingKey(key, b)
		if err != nil {
			return err
		}
		b = resolved
	}

	// Update the value in the DB.
	if err := a.core.UpdateSettingsByKey(key, b); err != nil {
		return err
	}

	return a.handleSettingsRestart(c)
}

// handleSettingsRestart checks for running campaigns and either triggers an
// immediate app restart or marks the app as needing a restart.
func (a *App) handleSettingsRestart(c echo.Context) error {
	// If there are any active campaigns, don't do an auto reload and
	// warn the user on the frontend.
	if a.manager.HasRunningCampaigns() {
		a.Lock()
		a.needsRestart = true
		a.Unlock()

		return c.JSON(http.StatusOK, okResp{struct {
			NeedsRestart bool `json:"needs_restart"`
		}{true}})
	}

	// No running campaigns. Reload the app.
	go func() {
		<-time.After(time.Millisecond * 500)
		a.chReload <- syscall.SIGHUP
	}()

	return c.JSON(http.StatusOK, okResp{true})
}

// GetLogs returns the log entries stored in the log buffer.
func (a *App) GetLogs(c echo.Context) error {
	return c.JSON(http.StatusOK, okResp{a.bufLog.Lines()})
}

// TestSMTPSettings returns the log entries stored in the log buffer.
func (a *App) TestSMTPSettings(c echo.Context) error {
	// Copy the raw JSON post body.
	reqBody, err := io.ReadAll(c.Request().Body)
	if err != nil {
		a.log.Printf("error reading SMTP test: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.internalError"))
	}

	// Load the JSON into koanf to parse SMTP settings properly including timestrings.
	ko := koanf.New(".")
	if err := ko.Load(rawbytes.Provider(reqBody), koanfjson.Parser()); err != nil {
		a.log.Printf("error unmarshalling SMTP test request: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.internalError"))
	}

	req := email.Server{}
	if err := ko.UnmarshalWithConf("", &req, koanf.UnmarshalConf{Tag: "json"}); err != nil {
		a.log.Printf("error scanning SMTP test request: %v", err)
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.internalError"))
	}

	to := ko.String("email")
	if to == "" {
		return echo.NewHTTPError(http.StatusBadRequest, a.i18n.Ts("globals.messages.missingFields", "name", "email"))
	}

	// Initialize a new SMTP pool.
	req.MaxConns = 1
	req.IdleTimeout = time.Second * 2
	req.PoolWaitTimeout = time.Second * 2
	msgr, err := email.New("", req)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest,
			a.i18n.Ts("globals.messages.errorCreating", "name", "SMTP", "error", err.Error()))
	}

	// Render the test email template body.
	var b bytes.Buffer
	if err := notifs.Tpls.ExecuteTemplate(&b, "smtp-test", nil); err != nil {
		a.log.Printf("error compiling notification template '%s': %v", "smtp-test", err)
		return err
	}

	m := models.Message{}
	m.From = a.cfg.FromEmail
	m.To = []string{to}
	m.Subject = a.i18n.T("settings.smtp.testConnection")
	m.Body = b.Bytes()
	if err := msgr.Push(m); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, okResp{a.bufLog.Lines()})
}

func (a *App) GetAboutInfo(c echo.Context) error {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	out := a.about
	out.System.AllocMB = mem.Alloc / 1024 / 1024
	out.System.OSMB = mem.Sys / 1024 / 1024

	return c.JSON(http.StatusOK, out)
}

// secretSettingKeys are the settings keys whose values carry secrets. PUT /api/settings/:key on
// any of them is resolved against the stored settings exactly like the full PUT (fork,
// 2026-09-03), so no client can persist the display mask through either route.
var secretSettingKeys = map[string]bool{
	"smtp":                            true,
	"bounce.mailboxes":                true,
	"messengers":                      true,
	"upload.s3.aws_secret_access_key": true,
	"bounce.sendgrid_key":             true,
	"bounce.azure":                    true,
	"bounce.postmark":                 true,
	"bounce.forwardemail":             true,
	"bounce.lettermint":               true,
	"security.captcha":                true,
	"security.oidc":                   true,
}

// resolveSecretSettingKey overlays one incoming settings key onto the stored settings, runs
// resolveSettingsSecrets over the result, and returns the resolved value of that key.
func (a *App) resolveSecretSettingKey(key string, incoming json.RawMessage) (json.RawMessage, error) {
	cur, err := a.core.GetSettings()
	if err != nil {
		return nil, err
	}

	// Stored settings -> map, overlay the key, -> models.Settings.
	curB, err := json.Marshal(cur)
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(curB, &m); err != nil {
		return nil, err
	}
	m[key] = incoming
	merged, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var set models.Settings
	if err := json.Unmarshal(merged, &set); err != nil {
		return nil, echo.NewHTTPError(http.StatusBadRequest, a.i18n.T("globals.messages.invalidData"))
	}

	if err := resolveSettingsSecrets(&set, cur); err != nil {
		return nil, err
	}

	// Resolved settings -> map, pick the key back out. A flat secret that resolved to ""
	// is omitted by its `omitempty` tag; "" is then the value to store.
	outB, err := json.Marshal(set)
	if err != nil {
		return nil, err
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(outB, &out); err != nil {
		return nil, err
	}
	v, ok := out[key]
	if !ok {
		v = json.RawMessage(`""`)
	}
	return v, nil
}

// resolveSettingsSecrets replaces every secret field in set with core.ResolveSecret's
// verdict against the stored value in cur (matched by UUID for the block arrays): empty or
// display-mask incoming keeps the stored secret, a value merely containing the mask is a
// 400, anything else is the new secret. An enabled SMTP/bounce block that authenticates,
// whose incoming password is the mask, and for which there is NO stored secret to keep
// (a new block, or a UUID that matches nothing) is refused rather than saved with an empty
// password -- the mask refers to a secret that does not exist.
func resolveSettingsSecrets(set *models.Settings, cur models.Settings) error {
	for i, s := range set.SMTP {
		var curPwd string
		for _, c := range cur.SMTP {
			if s.UUID == c.UUID {
				curPwd = c.Password
			}
		}
		pwd, err := core.ResolveSecret(s.Password, curPwd)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("SMTP #%d password: %v", i+1, err))
		}
		if pwd == "" && s.Enabled && s.AuthProtocol != "none" && core.IsSecretMask(s.Password) {
			return echo.NewHTTPError(http.StatusBadRequest,
				fmt.Sprintf("SMTP #%d password: the masked value refers to no stored secret for this block; enter the password", i+1))
		}
		set.SMTP[i].Password = pwd
	}

	for i, s := range set.BounceBoxes {
		var curPwd string
		for _, c := range cur.BounceBoxes {
			if s.UUID == c.UUID {
				curPwd = c.Password
			}
		}
		pwd, err := core.ResolveSecret(s.Password, curPwd)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("bounce mailbox #%d password: %v", i+1, err))
		}
		if pwd == "" && s.Enabled && s.AuthProtocol != "none" && core.IsSecretMask(s.Password) {
			return echo.NewHTTPError(http.StatusBadRequest,
				fmt.Sprintf("bounce mailbox #%d password: the masked value refers to no stored secret for this block; enter the password", i+1))
		}
		set.BounceBoxes[i].Password = pwd
	}

	for i, m := range set.Messengers {
		var curPwd string
		for _, c := range cur.Messengers {
			if m.UUID == c.UUID {
				curPwd = c.Password
			}
		}
		pwd, err := core.ResolveSecret(m.Password, curPwd)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("messenger #%d password: %v", i+1, err))
		}
		set.Messengers[i].Password = pwd
	}

	for _, f := range []struct {
		label string
		dst   *string
		cur   string
	}{
		{"upload.s3.aws_secret_access_key", &set.UploadS3AwsSecretAccessKey, cur.UploadS3AwsSecretAccessKey},
		{"bounce.sendgrid_key", &set.SendgridKey, cur.SendgridKey},
		{"bounce.azure.shared_secret", &set.BounceAzure.SharedSecret, cur.BounceAzure.SharedSecret},
		{"bounce.postmark.password", &set.BouncePostmark.Password, cur.BouncePostmark.Password},
		{"bounce.forwardemail.key", &set.BounceForwardEmail.Key, cur.BounceForwardEmail.Key},
		{"bounce.lettermint.key", &set.BounceLettermint.Key, cur.BounceLettermint.Key},
		{"security.captcha.hcaptcha.secret", &set.SecurityCaptcha.HCaptcha.Secret, cur.SecurityCaptcha.HCaptcha.Secret},
		{"security.oidc.client_secret", &set.OIDC.ClientSecret, cur.OIDC.ClientSecret},
	} {
		v, err := core.ResolveSecret(*f.dst, f.cur)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, fmt.Sprintf("%s: %v", f.label, err))
		}
		*f.dst = v
	}
	return nil
}
