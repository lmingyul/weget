package telegram

import "testing"

func TestParseURL(t *testing.T) {
	for _, tc := range []struct {
		url     string
		channel string
		private bool
	}{
		{"https://t.me/c/3018768304/15585", "", true},
		{"https://t.me/example_channel/15585?single", "example_channel", false},
		{"https://telegram.me/example_channel/15585", "example_channel", false},
		{"https://www.t.me/s/example_channel/15585", "example_channel", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			msg, err := ParseURL(tc.url)
			if err != nil {
				t.Fatal(err)
			}
			if msg.MessageID != 15585 || msg.IsPrivate != tc.private || msg.ChannelUsername != tc.channel {
				t.Fatalf("unexpected message: %+v", msg)
			}
			if tc.private && msg.ChannelID != 3018768304 {
				t.Fatalf("wrong private channel: %d", msg.ChannelID)
			}
		})
	}
	for _, rawURL := range []string{
		"https://example.com/t.me/channel/123",
		"https://t.me.example.com/channel/123",
		"https://t.me@evil.example/channel/123",
		"https://t.me/channel/0",
		"https://t.me/channel/2147483648",
		"https://t.me/c/0/123",
		"https://t.me/c/123",
		"https://t.me/channel/123/extra",
		"https://t.me/channel",
		"file://t.me/channel/123",
	} {
		if MatchURL(rawURL) {
			t.Errorf("accepted invalid message URL %q", rawURL)
		}
	}
}
