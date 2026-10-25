package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/guiyumin/vget/internal/core/extractor"
)

type playlistFixtureExtractor struct{}

func (playlistFixtureExtractor) Name() string { return "playlist-fixture" }
func (playlistFixtureExtractor) Match(u *url.URL) bool {
	return u.Hostname() == "playlist.fixture.test"
}
func (playlistFixtureExtractor) Extract(string) (extractor.Media, error) {
	return nil, errors.New("preview must not resolve media streams")
}
func (playlistFixtureExtractor) ExtractPlaylist(_ context.Context, rawURL string) (*extractor.Playlist, error) {
	if strings.Contains(rawURL, "fail") {
		return nil, errors.New("metadata unavailable")
	}
	if strings.Contains(rawURL, "single") {
		return nil, nil
	}
	return &extractor.Playlist{Title: "Fixture", Entries: []extractor.PlaylistEntry{{Index: 1}, {Index: 2}}}, nil
}

func TestDownloadPreviewDoesNotQueueJobs(t *testing.T) {
	extractor.Register(playlistFixtureExtractor{}, "playlist.fixture.test")
	srv := &Server{jobQueue: NewJobQueue(1, t.TempDir(), nil)}
	for _, tc := range []struct {
		name, rawURL string
		status       int
		parts        int
	}{
		{"multiple", "https://playlist.fixture.test/watch", 200, 2},
		{"single", "https://playlist.fixture.test/single", 200, 0},
		{"other_provider", "https://media.example/video.mp4", 200, 0},
		{"missing_url", "", 400, 0},
		{"metadata_failure", "https://playlist.fixture.test/fail", 502, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(DownloadRequest{URL: tc.rawURL})
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/download/preview", strings.NewReader(string(body)))
			c.Request.Header.Set("Content-Type", "application/json")
			srv.handleDownloadPreview(c)
			if recorder.Code != tc.status {
				t.Fatalf("unexpected preview status: %d %s", recorder.Code, recorder.Body.String())
			}
			if tc.status == http.StatusOK {
				var response struct {
					Data struct {
						Playlist *extractor.Playlist `json:"playlist"`
					} `json:"data"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				if tc.parts == 0 && response.Data.Playlist != nil {
					t.Fatal("single videos must not show a playlist")
				}
				if tc.parts > 0 && (response.Data.Playlist == nil || len(response.Data.Playlist.Entries) != tc.parts) {
					t.Fatal("preview lost episode entries")
				}
			}
			if len(srv.jobQueue.GetAllJobs()) != 0 {
				t.Fatal("preview queued downloads before user confirmation")
			}
		})
	}
}
