package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestTelegramDownloadRouting(t *testing.T) {
	srv := &Server{outputDir: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for _, url := range []string{
		"https://t.me/c/3018768304/15585",
		"https://t.me/example_channel/123",
		"https://telegram.me/example_channel/123",
	} {
		t.Run(url, func(t *testing.T) {
			err := srv.downloadWithExtractor(ctx, url, "", nil)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("Telegram download should honor cancellation, got %v", err)
			}
		})
	}

	t.Run("unrelated host", func(t *testing.T) {
		err := srv.downloadWithExtractor(ctx, "https://example.com/t.me/channel/123", "", nil)
		if err == nil || !strings.Contains(err.Error(), "no extractor found") {
			t.Fatalf("unrelated URL should use the regular extractor path, got %v", err)
		}
	})
}

func TestTelegramReturnFileRouting(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/download", nil).WithContext(ctx)
	srv := &Server{outputDir: t.TempDir()}
	srv.downloadAndStream(c, "https://t.me/c/3018768304/15585", "")
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "context canceled") {
		t.Fatalf("Telegram return_file did not reach the authenticated downloader: %d %s", recorder.Code, recorder.Body.String())
	}
}
