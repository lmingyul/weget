package extractor

import (
	"context"
	"fmt"
	"net/url"

	"github.com/guiyumin/vget/internal/core/config"
)

func (b *BilibiliExtractor) ExtractPlaylist(ctx context.Context, rawURL string) (*Playlist, error) {
	worker := *b
	worker.cookie = config.LoadOrDefault().Bilibili.Cookie
	return worker.extractPlaylist(ctx, rawURL)
}

func (b *BilibiliExtractor) extractPlaylist(ctx context.Context, rawURL string) (*Playlist, error) {
	normalized, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(normalized)
	if err != nil || !b.Match(u) {
		return nil, fmt.Errorf("invalid Bilibili video URL")
	}
	b.initClient()
	aid, bvid, page, err := b.resolveVideoID(ctx, normalized)
	if err != nil {
		return nil, err
	}
	info, err := b.fetchVideoInfo(ctx, aid)
	if err != nil {
		return nil, err
	}
	if page > len(info.Pages) {
		return nil, fmt.Errorf("bilibili page %d out of range: video has %d pages", page, len(info.Pages))
	}
	if len(info.Pages) < 2 {
		return nil, nil
	}
	partURL := func(index int) string {
		return fmt.Sprintf("https://www.bilibili.com/video/%s/?p=%d", bvid, index)
	}
	playlist := &Playlist{Title: info.Title, CurrentURL: partURL(page)}
	for i, part := range info.Pages {
		title := part.Part
		if title == "" {
			title = fmt.Sprintf("P%d", i+1)
		}
		playlist.Entries = append(playlist.Entries, PlaylistEntry{
			Index: i + 1, Title: title, URL: partURL(i + 1), Current: i+1 == page,
		})
	}
	return playlist, nil
}
