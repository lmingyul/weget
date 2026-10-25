package telegram

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Message identifies a post in a public or private Telegram channel.
type Message struct {
	ChannelUsername string
	ChannelID       int64
	MessageID       int
	IsPrivate       bool
}

func ParseURL(rawURL string) (*Message, error) {
	u, err := url.Parse(rawURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil {
		return nil, fmt.Errorf("invalid Telegram message URL")
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if host != "t.me" && host != "telegram.me" {
		return nil, fmt.Errorf("invalid Telegram host")
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 3 && parts[0] == "s" {
		parts = parts[1:]
	}
	msg := &Message{}
	switch {
	case len(parts) == 3 && parts[0] == "c":
		msg.IsPrivate = true
		msg.ChannelID, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || msg.ChannelID <= 0 {
			return nil, fmt.Errorf("invalid Telegram channel ID")
		}
	case len(parts) == 2 && parts[0] != "c" && parts[0] != "s":
		msg.ChannelUsername = parts[0]
		for _, c := range msg.ChannelUsername {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
				return nil, fmt.Errorf("invalid Telegram channel username")
			}
		}
	default:
		return nil, fmt.Errorf("expected a Telegram message link, such as https://t.me/channel/123")
	}
	id, err := strconv.ParseInt(parts[len(parts)-1], 10, 32)
	if err != nil || id <= 0 {
		return nil, fmt.Errorf("invalid Telegram message ID")
	}
	msg.MessageID = int(id)
	return msg, nil
}

func MatchURL(rawURL string) bool {
	_, err := ParseURL(rawURL)
	return err == nil
}
