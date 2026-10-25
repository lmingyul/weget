package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guiyumin/vget/internal/core/downloader"
	"github.com/guiyumin/vget/internal/core/extractor"
)

func TestDASHFallbacksRecoverCDNFailures(t *testing.T) {
	dir := writeDASHFixtures(t)
	files := http.FileServer(http.Dir(dir))
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") != "https://www.bilibili.com/" || r.Header.Get("User-Agent") != "fallback-fixture" {
			t.Error("CDN request lost its download headers")
		}
		switch r.URL.Path {
		case "/eof":
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_ = conn.Close()
		case "/truncated":
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("partial"))
		case "/unavailable":
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			files.ServeHTTP(w, r)
		}
	}))
	defer media.Close()

	for _, failure := range []string{"audio EOF", "truncated video"} {
		t.Run(failure, func(t *testing.T) {
			videoURL, audioURL := media.URL+"/video.mp4", media.URL+"/audio.m4a"
			if failure == "audio EOF" {
				audioURL = media.URL + "/eof"
			} else {
				videoURL = media.URL + "/truncated"
			}
			data, err := json.Marshal(map[string]any{
				"URL": videoURL, "AudioURL": audioURL, "Ext": "mp4",
				"BackupURLs":      []string{media.URL + "/video.mp4"},
				"AudioBackupURLs": []string{media.URL + "/unavailable", media.URL + "/audio.m4a"},
				"Headers":         map[string]string{"Referer": "https://www.bilibili.com/", "User-Agent": "fallback-fixture"},
			})
			if err != nil {
				t.Fatal(err)
			}
			var format extractor.VideoFormat
			if err := json.Unmarshal(data, &format); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			output := filepath.Join(t.TempDir(), "output.mp4")
			if err := (&Server{}).downloadVideoWithAudio(ctx, &format, output, nil); err != nil {
				t.Fatalf("CDN fallback did not recover: %v", err)
			}
			streams, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "stream=codec_type", "-of", "csv=p=0", output).CombinedOutput()
			if err != nil || !strings.Contains(string(streams), "video") || !strings.Contains(string(streams), "audio") {
				t.Fatalf("fallback output must contain video and audio: %v, %s", err, streams)
			}
		})
	}
}

func TestDownloadFallbackResetsPartialFileAndProgress(t *testing.T) {
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/partial" {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("partial"))
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer media.Close()
	ctx := downloader.WithConcurrencyTracking(context.Background())
	output := filepath.Join(t.TempDir(), "stream.bin")
	var progress [][2]int64
	err := downloadFileWithFallbacks(ctx, media.URL+"/partial", []string{media.URL + "/backup"}, output, nil, func(downloaded, total int64) {
		progress = append(progress, [2]int64{downloaded, total})
		if count := downloader.ActiveTransfers(ctx); count != 1 {
			t.Errorf("sequential fallback reports %d active transfers, want 1", count)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "ok" {
		t.Fatalf("fallback retained partial data: %q, %v", data, err)
	}
	if len(progress) < 4 || progress[1] != [2]int64{7, 100} || progress[2] != [2]int64{0, 2} || progress[len(progress)-1] != [2]int64{2, 2} {
		t.Fatalf("fallback did not reset progress: %v", progress)
	}
	if count := downloader.ActiveTransfers(ctx); count != 0 {
		t.Errorf("completed fallback left %d active transfers", count)
	}
}
