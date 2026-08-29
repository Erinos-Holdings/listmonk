package core

import (
	"testing"
	"time"

	"github.com/knadh/listmonk/models"
	null "gopkg.in/volatiletech/null.v6"
)

func TestLangLockedChange(t *testing.T) {
	started := models.Campaign{CampaignMeta: models.CampaignMeta{StartedAt: null.TimeFrom(time.Now())}}
	draft := models.Campaign{}
	cases := []struct {
		name    string
		cm      models.Campaign
		prev    string
		attribs models.JSON
		locked  bool
	}{
		{"draft may change", draft, "fr", models.JSON{"lang": "en"}, false},
		{"started same lang", started, "fr", models.JSON{"lang": "fr"}, false},
		{"started other lang", started, "fr", models.JSON{"lang": "en"}, true},
		{"started clear lang", started, "fr", models.JSON{"preheader": "x"}, true},
		{"started set lang on everyone", started, "", models.JSON{"lang": "de"}, true},
		{"started omits attribs keeps stored", started, "fr", nil, false},
		{"started everyone stays everyone", started, "", models.JSON{"preheader": "x"}, false},
	}
	for _, c := range cases {
		if got := LangLockedChange(c.cm, c.prev, c.attribs); got != c.locked {
			t.Errorf("%s: got %v want %v", c.name, got, c.locked)
		}
	}
}
