// Package joinpage is the fork's join-link page policy (integrations listmonk/JOIN-LINK-SPEC
// D2/D10): which page a join_plan outcome renders, the page language, the brand's display name
// and the copy in the five campaign languages. Pure, so it is unit tested here; the handlers in
// cmd/join.go only wire it.
package joinpage

import (
	"net/mail"
	"strings"
)

// The three states of the join page (D2) and the result page.
const (
	StateConfirm = "confirm"
	StateAlready = "already"
	StateCannot  = "cannot"
	StateJoined  = "joined"
)

// State maps a join_plan outcome to the page it renders: join -> the Confirm form, already ->
// Already subscribed, every other outcome (no_campaign, no_brand, no_master, refused, cannot, or
// anything unknown) -> the generic Cannot page, which never names a reason (D2/D11).
func State(outcome string) string {
	switch outcome {
	case "join":
		return StateConfirm
	case "already":
		return StateAlready
	default:
		return StateCannot
	}
}

// Langs is the closed set of page languages (the campaign languages).
var Langs = []string{"en", "es", "fr", "de", "it"}

// PageLang is D10's page language: the plan's language (the campaign's) when set, else the
// subscriber's attribs.lang; its primary subtag must be one of Langs, else "en".
func PageLang(planLang, subLang string) string {
	l := strings.TrimSpace(planLang)
	if l == "" {
		l = strings.TrimSpace(subLang)
	}
	primary := strings.ToLower(strings.SplitN(strings.ReplaceAll(l, "_", "-"), "-", 2)[0])
	for _, s := range Langs {
		if primary == s {
			return s
		}
	}
	return "en"
}

// BrandName is the brand as the page names it: the display-name part of the brands row's
// from_email ("Shala <hello@shala.com>" -> "Shala"), else the slug.
func BrandName(fromEmail, slug string) string {
	if a, err := mail.ParseAddress(strings.TrimSpace(fromEmail)); err == nil {
		if n := strings.TrimSpace(a.Name); n != "" {
			return n
		}
	}
	return slug
}

// Copy is the page text in one language, {brand} and {email} already substituted.
type Copy struct {
	Heading     string
	Body        string
	Button      string
	DoneHeading string
	DoneBody    string
	Already     string
	Cannot      string
}

// copies is D10's table verbatim ({brand} and {email} placeholders). A silent join: no copy
// promises a welcome.
var copies = map[string]Copy{
	"en": {
		Heading:     "Get {brand} emails",
		Body:        "Confirm below to receive {brand} emails at {email}.",
		Button:      "Confirm",
		DoneHeading: "You're subscribed",
		DoneBody:    "You'll receive {brand} emails at {email}.",
		Already:     "{email} already receives {brand} emails.",
		Cannot:      "We couldn't update your subscription from this link.",
	},
	"fr": {
		Heading:     "Recevez les e-mails de {brand}",
		Body:        "Confirmez ci-dessous pour recevoir les e-mails de {brand} à l'adresse {email}.",
		Button:      "Confirmer",
		DoneHeading: "C'est confirmé",
		DoneBody:    "Vous recevrez les e-mails de {brand} à l'adresse {email}.",
		Already:     "L'adresse {email} reçoit déjà les e-mails de {brand}.",
		Cannot:      "Nous n'avons pas pu mettre à jour votre abonnement à partir de ce lien.",
	},
	"es": {
		Heading:     "Recibe los correos de {brand}",
		Body:        "Confirma abajo para recibir los correos de {brand} en {email}.",
		Button:      "Confirmar",
		DoneHeading: "¡Listo!",
		DoneBody:    "Recibirás los correos de {brand} en {email}.",
		Already:     "{email} ya recibe los correos de {brand}.",
		Cannot:      "No pudimos actualizar tu suscripción desde este enlace.",
	},
	"de": {
		Heading:     "E-Mails von {brand} erhalten",
		Body:        "Bestätigen Sie unten, dass Sie E-Mails von {brand} an {email} erhalten möchten.",
		Button:      "Bestätigen",
		DoneHeading: "Bestätigt",
		DoneBody:    "Sie erhalten E-Mails von {brand} an {email}.",
		Already:     "{email} erhält bereits E-Mails von {brand}.",
		Cannot:      "Ihr Abonnement konnte über diesen Link nicht aktualisiert werden.",
	},
	"it": {
		Heading:     "Ricevi le email di {brand}",
		Body:        "Conferma qui sotto per ricevere le email di {brand} all'indirizzo {email}.",
		Button:      "Conferma",
		DoneHeading: "Fatto",
		DoneBody:    "Riceverai le email di {brand} all'indirizzo {email}.",
		Already:     "{email} riceve già le email di {brand}.",
		Cannot:      "Non è stato possibile aggiornare la tua iscrizione da questo link.",
	},
}

// For returns the copy in lang (normalized by PageLang's rule; an unknown language is English)
// with brand and email substituted. The values are plain text -- the template escapes them.
func For(lang, brand, email string) Copy {
	c, ok := copies[PageLang(lang, "")]
	if !ok {
		c = copies["en"]
	}
	r := strings.NewReplacer("{brand}", brand, "{email}", email)
	return Copy{
		Heading:     r.Replace(c.Heading),
		Body:        r.Replace(c.Body),
		Button:      r.Replace(c.Button),
		DoneHeading: r.Replace(c.DoneHeading),
		DoneBody:    r.Replace(c.DoneBody),
		Already:     r.Replace(c.Already),
		Cannot:      r.Replace(c.Cannot),
	}
}
