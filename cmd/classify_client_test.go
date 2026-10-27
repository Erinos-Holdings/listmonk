package main

// Fork (client stats) -- integrations CLIENT-STATS-SPEC I1: classifyClient is total and returns
// only a vocabulary token or "" (stored NULL), with first-match order proxies, named mail clients
// (incl. outlook-mobile), Apple Mail's terminal signature, then browsers. At least one real-world
// UA per token; the review F1 must-not-be-apple-mail rows; the review F2 Outlook mobile rows.

import (
	"math/rand"
	"strings"
	"testing"
)

// clientVocabulary is the closed token set (D2). Extending it means touching classifyClient, this
// table and frontend/src/clientRows.mjs in the same change.
var clientVocabulary = map[string]bool{
	clientGmailProxy: true, clientYahoo: true, clientOutlookWindows: true, clientOutlookMac: true,
	clientThunderbird: true, clientOutlookMobile: true, clientAppleMail: true, clientBrowserIOS: true,
	clientBrowserAndroid: true, clientBrowserWindows: true, clientBrowserMac: true,
	clientBrowserLinux: true,
	// integrations CLIENT-VOCABULARY-SPEC D2 -- the four appended tokens.
	clientUnfurler: true, clientAutomation: true, clientWebviewIOS: true,
	clientBareMozilla: true,
	clientOther:       true,
}

func TestClassifyClient(t *testing.T) {
	cases := []struct{ ua, want string }{
		// Image proxies.
		{"Mozilla/5.0 (Windows NT 5.1; rv:11.0) Gecko Firefox/11.0 (via ggpht.com GoogleImageProxy)", clientGmailProxy},
		{"YahooMailProxy; https://help.yahoo.com/kb/yahoo-mail-proxy-SLN28749.html", clientYahoo},
		{"Mozilla/5.0 (compatible; Yahoo Mailproxy)", clientYahoo},

		// Named mail clients.
		{"Microsoft Office/16.0 (Windows NT 10.0; Microsoft Outlook 16.0.17928; Pro)", clientOutlookWindows},
		{"Mozilla/4.0 (compatible; MSIE 7.0; Windows NT 10.0; WOW64; Trident/7.0; .NET4.0C; .NET4.0E; ms-office; MSOffice 16)", clientOutlookWindows},
		{"Outlook-Mac/16.89 (Macintosh; Mac OS X 14.6)", clientOutlookMac},
		// Review Low 2 -- the real-world Outlook-for-Mac form has no "Macintosh" token.
		{"MacOutlook/16.70.23021201 (Intelx64 Mac OS X 13.2.1)", clientOutlookMac},
		{"Microsoft Office/16.0 (Macintosh; Mac OS X 14.6; Microsoft Outlook 16.89)", clientOutlookMac},
		{"Mozilla/5.0 (X11; Linux x86_64; rv:128.0) Gecko/20100101 Thunderbird/128.3.1", clientThunderbird},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:115.0) Gecko/20100101 Thunderbird/115.15.0", clientThunderbird},
		{"Outlook-iOS/723.4027091.prod.iphone (4.2421.0)", clientOutlookMobile},
		{"Outlook-Android/2.0", clientOutlookMobile},
		{"Mozilla/5.0 (Linux; Android 14; Pixel 8 Build/AP2A; wv) Outlook-Android/2.0", clientOutlookMobile},

		// Apple Mail / MPP: the UA terminates at (KHTML, like Gecko), optionally + Mobile/<build>.
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko)", clientAppleMail},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148", clientAppleMail},
		{"Mozilla/5.0 (iPad; CPU OS 16_7 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148", clientAppleMail},

		// Review F1: NOT apple-mail -- a further product token means a browser or an in-app WebView.
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) FxiOS/127.0 Mobile/15E148 Safari/605.1.15", clientBrowserIOS},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1 DuckDuckGo/7", clientBrowserIOS},
		// CLIENT-VOCABULARY-SPEC D2/J4: these two were the recorded `other` gap and are now named
		// webview-ios (iPhone + AppleWebKit, no Safari/CriOS token).
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/21F90 [FBAN/FBIOS;FBAV/470.0.0.43.109;FBBV/612345678;FBDV/iPhone15,2;FBMD/iPhone;FBSN/iOS;FBSV/17.5;FBSS/3;FBCR/;FBID/phone;FBLC/en_US;FBOP/80]", clientWebviewIOS},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/21F90 Instagram 340.0.0.22.109 (iPhone15,2; iOS 17_5; en_US; en; scale=3.00; 1179x2556; 621234567)", clientWebviewIOS},

		// Browsers by platform.
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Mobile/15E148 Safari/604.1", clientBrowserIOS},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) CriOS/126.0.6478.153 Mobile/15E148 Safari/604.1", clientBrowserIOS},
		{"Mozilla/5.0 (Linux; Android 14; SM-S918B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36", clientBrowserAndroid},
		{"Mozilla/5.0 (Linux; Android 13; SM-G991U Build/TP1A; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/125.0.6422.165 Mobile Safari/537.36", clientBrowserAndroid},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 Edg/126.0.0.0", clientBrowserWindows},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:128.0) Gecko/20100101 Firefox/128.0", clientBrowserWindows},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.5 Safari/605.1.15", clientBrowserMac},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36", clientBrowserMac},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 14.5; rv:128.0) Gecko/20100101 Firefox/128.0", clientBrowserMac},
		{"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36", clientBrowserLinux},
		{"Mozilla/5.0 (X11; Ubuntu; Linux x86_64; rv:128.0) Gecko/20100101 Firefox/128.0", clientBrowserLinux},

		// --- integrations CLIENT-VOCABULARY-SPEC ---

		// Declared automation (D2/D3). The first three were `other` before this vocabulary.
		{"curl/8.5.0", clientAutomation},
		{"python-requests/2.32.3", clientAutomation},
		{"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", clientAutomation},
		{"Wget/1.21.4", clientAutomation},
		{"Go-http-client/2.0", clientAutomation},
		{"PostmanRuntime/7.39.0", clientAutomation},
		{"Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)", clientAutomation},
		{"Mozilla/5.0 (compatible; SemrushBot/7~bl; +http://www.semrush.com/bot.html)", clientAutomation},
		{"Some-Scanner/1.0 (proofpoint)", clientAutomation},
		// J2 order pin: a declared robot carrying a platform token resolves to automation, NOT
		// browser-* (D4.1 -- these were browser-windows / browser-android / browser-mac before).
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/126.0.0.0 Safari/537.36", clientAutomation},
		{"Mozilla/5.0 (Linux; Android 7.0;) AppleWebKit/537.36 (KHTML, like Gecko) Mobile Safari/537.36 (compatible; PetalBot;+https://webmaster.petalsearch.com/site/petalbot)", clientAutomation},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_5) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/13.1.1 Safari/605.1.15 (Applebot/0.1; +http://www.apple.com/go/applebot)", clientAutomation},
		{"Mozilla/5.0 (compatible; YandexBot/3.0; +http://yandex.com/bots)", clientAutomation},

		// Declared link-preview robots (D2), matched BEFORE automation and before the browsers.
		{"facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)", clientUnfurler},
		{"Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)", clientUnfurler},
		{"Mozilla/5.0 (compatible; LinkedInBot/1.0; +http://www.linkedin.com)", clientUnfurler},
		{"Twitterbot/1.0", clientUnfurler},
		{"TelegramBot (like TwitterBot)", clientUnfurler},
		{"Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com)", clientUnfurler},
		{"SkypeUriPreview Preview/0.5", clientUnfurler},
		{"Mozilla/5.0 (compatible; redditbot/1.0; +http://www.reddit.com/feedback)", clientUnfurler},
		{"Pinterest/0.2 (+http://www.pinterest.com/bot.html; pinterestbot)", clientUnfurler},
		{"Embedly/0.1", clientUnfurler},
		{"Iframely/1.3.1", clientUnfurler},
		// J2 order pin: an unfurler wearing a Windows browser UA is still an unfurler.
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36 facebookexternalhit/1.1", clientUnfurler},

		// In-app browser on iOS (D2/J4): AppleWebKit on an iPhone/iPad/iPod with no Safari/CriOS.
		{"Mozilla/5.0 (iPad; CPU OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/15E148 [LinkedInApp]", clientWebviewIOS},
		// M1: a Mac generic-WebKit UA with a trailing product token is NOT an iOS WebView.
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/", clientOther},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) MyMacApp/1.0", clientOther},

		// Bare Mozilla (D2/J3): begins with "mozilla", volunteers no platform and no engine token.
		{"Mozilla/5.0", clientBareMozilla},
		{"Mozilla", clientBareMozilla},
		{"Mozilla/5.0 (compatible)", clientBareMozilla},
		// L1: NOT bare-mozilla -- MSIE is volunteered, and the second does not begin with Mozilla.
		{"Mozilla/4.0 (compatible; MSIE 6.0)", clientOther},
		{"MailSniper/1.0 Mozilla", clientOther},

		// D5/D3's REJECTED signatures, pinned so a later edit cannot quietly admit them: a generic
		// HTTP-library default is not a declaration of automation (a native mail app loading images
		// outside a WebView sends exactly these, and `machine` would report real opens as
		// machines), `BingPreview` is attested only by third-party directories so it stays in the
		// declared browser-windows soft edge, and `whatsapp` conflates a preview robot with a
		// person reading inside an in-app browser.
		{"okhttp/4.12.0", clientOther},
		{"Java/17.0.2", clientOther},
		{"Mozilla/5.0 (Windows NT 6.1; WOW64) AppleWebKit/534+ (KHTML, like Gecko) BingPreview/1.0b", clientBrowserWindows},
		{"Dalvik/2.1.0 (Linux; U; Android 14; SM-S918B Build/UP1A)", clientBrowserAndroid},
		{"WhatsApp/2.23.20.0 A", clientOther},

		// Case-insensitive.
		{"googleimageproxy", clientGmailProxy},
		{"OUTLOOK-IOS/1.0", clientOutlookMobile},
		{"MOZILLA/5.0", clientBareMozilla},
		{"CURL/8.5.0", clientAutomation},

		// Empty and junk -> "" (stored NULL), never a panic.
		{"", ""},
		{"   \t ", ""},
		{"\x00\x01\x02", ""},
		{"Mozilla/5.0\x00 (Windows NT 10.0)", ""},
		{"\xff\xfe\xfd", ""},
		{"Mozilla/5.0 (Ünïcode)", ""},

		// Review M1: a self-declaring "(compatible; <Bot>/<v>; +URL)" crawler carries no platform
		// and no engine token, but it is the OPPOSITE of bare -- it must not contaminate the one
		// bucket that tests the Apple hypothesis (isBareRemainder).
		{"Mozilla/5.0 (compatible; MJ12bot/v1.4.8; http://mj12bot.com/)", clientOther},
		{"Mozilla/5.0 zgrab/0.x", clientOther},
		{"Mozilla/5.0 (compatible; MailChimp)", clientOther},
		{"Mozilla/5.0 (compatible; Yahoo! Slurp; http://help.yahoo.com/help/us/ysearch/slurp)", clientOther},
		{"12345 / ; ()", ""},
		{"Mozilla/5.0 (Macintosh) AppleWebKit/605 (KHTML, like Gecko)\n", clientAppleMail},
	}
	// Every row is its own regression pin: integrations CLIENT-VOCABULARY-SPEC J3/J5 -- apart from
	// the spec's exhaustive changed-row list (exactly five: curl, python-requests and bingbot ->
	// automation; the FBAN and Instagram rows -> webview-ios; the Macintosh Mobile/ row
	// unchanged), every pre-existing row keeps the token it asserted before the vocabulary grew.
	for _, c := range cases {
		got := classifyClient(c.ua)
		if got != c.want {
			t.Errorf("classifyClient(%q) = %q, want %q", c.ua, got, c.want)
		}
		if got != "" && !clientVocabulary[got] {
			t.Errorf("classifyClient(%q) = %q, not a vocabulary token", c.ua, got)
		}
	}

	// J5 / D5's collision rule, asserted directly: no substring of the two DECLARED lists occurs
	// inside any UA in this table that is not itself a declared robot. This fails the moment a new
	// signature is added that could swallow a real client's traffic.
	for _, c := range cases {
		if c.want == clientAutomation || c.want == clientUnfurler {
			continue
		}
		s := strings.ToLower(strings.TrimSpace(c.ua))
		if containsAny(s, clientAutomationSigs) {
			t.Errorf("D5 collision: automation signature matches non-robot UA %q (want %q)", c.ua, c.want)
		}
		if containsAny(s, clientUnfurlerSigs) {
			t.Errorf("D5 collision: unfurler signature matches non-robot UA %q (want %q)", c.ua, c.want)
		}
	}

	// Every token is produced by at least one case above.
	seen := map[string]bool{}
	for _, c := range cases {
		seen[c.want] = true
	}
	for tok := range clientVocabulary {
		if !seen[tok] {
			t.Errorf("no test case produces token %q", tok)
		}
	}
}

// TestClassifyClientTotal throws random byte strings (and random fragments of real signatures) at
// the classifier: it must never panic and must return a vocabulary token or "".
func TestClassifyClientTotal(t *testing.T) {
	// The fragment list covers every rule family, the vocabulary extension's included: the
	// declared-robot signatures and the "(compatible;" / contact-URL shapes isBareRemainder
	// inspects. It REACHES that predicate rather than exercising it heavily -- a random byte is
	// outside 0x20..0x7e 63% of the time, so most multi-piece UAs are rejected by the junk gate
	// first (Stage 4b review L1 counted the branches). "mail" is a letters-only fragment, so the
	// with-space-and-bare branch is at least reachable; both directions of the narrowing are
	// pinned by the table rows above, and this loop is here for totality, not for coverage.
	frags := []string{"(KHTML, like Gecko)", "Mobile/", "AppleWebKit/", "iPhone", "Macintosh", "Outlook", "Android", " ", "\x00", "\xff", "Mozilla/", "Safari", "/", "curl/", "(compatible", ";", "+http://", "@", "facebookexternalhit", "crawler", "mail"}
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 20000; i++ {
		var b strings.Builder
		n := r.Intn(12)
		for j := 0; j < n; j++ {
			if r.Intn(2) == 0 {
				b.WriteString(frags[r.Intn(len(frags))])
			} else {
				b.WriteByte(byte(r.Intn(256)))
			}
		}
		ua := b.String()
		func() {
			defer func() {
				if p := recover(); p != nil {
					t.Fatalf("classifyClient(%q) panicked: %v", ua, p)
				}
			}()
			if got := classifyClient(ua); got != "" && !clientVocabulary[got] {
				t.Fatalf("classifyClient(%q) = %q, not a vocabulary token", ua, got)
			}
		}()
	}
	// A very long UA. "Mozilla/5.0 " repeated volunteers no platform and no engine token, but its
	// remainder past the first space carries a "/", so it is not bare: `other`, as before the
	// vocabulary grew (integrations CLIENT-VOCABULARY-SPEC D2, narrowed at Stage 4 by review M1 --
	// the Stage 3 ratification that made this row bare-mozilla is reverted with it).
	if got := classifyClient(strings.Repeat("Mozilla/5.0 ", 100000)); got != clientOther {
		t.Fatalf("long UA = %q, want other", got)
	}
	// A megabyte of a single repeated platform token still terminates and stays in vocabulary.
	if got := classifyClient(strings.Repeat("x", 1<<20)); got != clientOther {
		t.Fatalf("1MB UA = %q, want other", got)
	}
}
