package extractor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBilibiliPlaylist(t *testing.T) {
	const base = "https://www.bilibili.com/video/BV1vVnA6XEEa/"
	for _, tc := range []struct {
		name, rawURL string
		single       bool
		current      int
		wantErr      bool
	}{
		{"all_parts", base, false, 1, false},
		{"selected_part", base + "?p=2&spm_id_from=fixture", false, 2, false},
		{"AV_link", "https://www.bilibili.com/video/av117473697333496/?p=3", false, 3, false},
		{"short_link", "https://b23.tv/example123", false, 2, false},
		{"single_video", base, true, 1, false},
		{"out_of_range", base + "?p=4", false, 0, true},
		{"invalid_part", base + "?p=0", false, 0, true},
		{"invalid_host", "https://example.com/video/BV1vVnA6XEEa/", false, 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ext := &BilibiliExtractor{cookie: "SESSDATA=playlist-fixture"}
			ext.client = &http.Client{
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
				Transport: bilibiliTransport(func(req *http.Request) (*http.Response, error) {
					header := make(http.Header)
					status := http.StatusOK
					body := `{"code":0,"data":{"title":"Playlist fixture","pages":[{"part":"Introduction"},{"part":"Setup"},{"part":"First project"}]}}`
					if req.URL.Host == "b23.tv" {
						if req.Header.Get("Cookie") != "" {
							t.Error("cookie sent to short-link resolver")
						}
						status = http.StatusFound
						header.Set("Location", base+"?p=2")
					} else {
						if req.URL.Host != "api.bilibili.com" || req.URL.Path != "/x/web-interface/view" {
							t.Errorf("preview must only request metadata, got %s", req.URL.Path)
						}
						if req.Header.Get("Cookie") != "SESSDATA=playlist-fixture" || req.Header.Get("Referer") != "https://www.bilibili.com/" {
							t.Error("metadata request lost authentication or Referer")
						}
					}
					if tc.single {
						body = `{"code":0,"data":{"title":"Single video","pages":[{"part":"Only part"}]}}`
					}
					return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
				}),
			}
			playlist, err := ext.extractPlaylist(context.Background(), tc.rawURL)
			if tc.wantErr {
				if err == nil {
					t.Fatal("invalid playlist URL was accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.single {
				if playlist != nil {
					t.Fatal("a single video must not trigger a playlist prompt")
				}
				return
			}
			if playlist == nil || len(playlist.Entries) != 3 || playlist.Title != "Playlist fixture" {
				t.Fatalf("wrong playlist: %+v", playlist)
			}
			for i, entry := range playlist.Entries {
				if entry.Index != i+1 || entry.Current != (i+1 == tc.current) || entry.Title == "" {
					t.Errorf("wrong episode metadata: %+v", entry)
				}
				if entry.Current && playlist.CurrentURL != entry.URL {
					t.Error("single-episode choice does not preserve the requested part")
				}
				if !strings.HasPrefix(entry.URL, base+"?p=") {
					t.Errorf("episode URL is not canonical: %s", entry.URL)
				}
			}
		})
	}
}

func TestBilibiliPlaylistCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ext := &BilibiliExtractor{client: &http.Client{Transport: bilibiliTransport(func(req *http.Request) (*http.Response, error) {
		return nil, req.Context().Err()
	})}}
	_, err := ext.extractPlaylist(ctx, "https://www.bilibili.com/video/BV1vVnA6XEEa/")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("metadata request lost cancellation: %v", err)
	}
}
