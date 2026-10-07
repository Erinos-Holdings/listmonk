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
	clientBrowserLinux: true, clientOther: true,
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
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/21F90 [FBAN/FBIOS;FBAV/470.0.0.43.109;FBBV/612345678;FBDV/iPhone15,2;FBMD/iPhone;FBSN/iOS;FBSV/17.5;FBSS/3;FBCR/;FBID/phone;FBLC/en_US;FBOP/80]", clientOther},
		{"Mozilla/5.0 (iPhone; CPU iPhone OS 17_5 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/21F90 Instagram 340.0.0.22.109 (iPhone15,2; iOS 17_5; en_US; en; scale=3.00; 1179x2556; 621234567)", clientOther},

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

		// Seen but not classified.
		{"curl/8.5.0", clientOther},
		{"python-requests/2.32.3", clientOther},
		{"Mozilla/5.0 (compatible; bingbot/2.0; +http://www.bing.com/bingbot.htm)", clientOther},
		{"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Mobile/", clientOther},

		// Case-insensitive.
		{"googleimageproxy", clientGmailProxy},
		{"OUTLOOK-IOS/1.0", clientOutlookMobile},

		// Empty and junk -> "" (stored NULL), never a panic.
		{"", ""},
		{"   \t ", ""},
		{"\x00\x01\x02", ""},
		{"Mozilla/5.0\x00 (Windows NT 10.0)", ""},
		{"\xff\xfe\xfd", ""},
		{"Mozilla/5.0 (Ünïcode)", ""},
		{"12345 / ; ()", ""},
		{"Mozilla/5.0 (Macintosh) AppleWebKit/605 (KHTML, like Gecko)\n", clientAppleMail},
	}
	for _, c := range cases {
		got := classifyClient(c.ua)
		if got != c.want {
			t.Errorf("classifyClient(%q) = %q, want %q", c.ua, got, c.want)
		}
		if got != "" && !clientVocabulary[got] {
			t.Errorf("classifyClient(%q) = %q, not a vocabulary token", c.ua, got)
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
	frags := []string{"(KHTML, like Gecko)", "Mobile/", "AppleWebKit/", "iPhone", "Macintosh", "Outlook", "Android", " ", "\x00", "\xff", "Mozilla/", "Safari", "/"}
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
	// A very long UA.
	if got := classifyClient(strings.Repeat("Mozilla/5.0 ", 100000)); got != clientOther {
		t.Fatalf("long UA = %q, want other", got)
	}
}
