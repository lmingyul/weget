package server

import (
	"context"
	"mime"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guiyumin/vget/internal/core/downloader"
	"github.com/guiyumin/vget/internal/core/extractor"
)

type dashFixtureExtractor struct{ videoURL, audioURL, title string }

func (d dashFixtureExtractor) Name() string { return "dash-fixture" }
func (d dashFixtureExtractor) Match(u *url.URL) bool {
	return u.Hostname() == "dash.fixture.test"
}
func (d dashFixtureExtractor) Extract(string) (extractor.Media, error) {
	return &extractor.VideoMedia{ID: "fixture", Title: d.title, Formats: []extractor.VideoFormat{{
		URL: d.videoURL, AudioURL: d.audioURL, Ext: "mp4", Height: 16, Bitrate: 1,
	}}}, nil
}

func TestDASHReturnFileMergesStreams(t *testing.T) {
	if !downloader.FFmpegAvailable() {
		t.Skip("ffmpeg is required for the DASH integration fixture")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is required to inspect the response streams")
	}
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fixtures := []struct {
		name string
		args []string
	}{
		{"video.mp4", []string{"-f", "lavfi", "-i", "color=c=black:s=16x16:r=1", "-frames:v", "1", "-c:v", "mpeg4", "-an"}},
		{"audio.m4a", []string{"-f", "lavfi", "-i", "sine=frequency=440", "-t", "1", "-c:a", "aac", "-vn"}},
	}
	for _, fixture := range fixtures {
		args := append([]string{"-nostdin", "-hide_banner", "-loglevel", "error"}, fixture.args...)
		args = append(args, filepath.Join(dir, fixture.name))
		if output, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
			t.Fatalf("create %s: %v\n%s", fixture.name, err, output)
		}
	}
	media := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer media.Close()
	outputDir := t.TempDir()
	srv := &Server{outputDir: outputDir}
	longASCII, longCJK := strings.Repeat("x", 57), strings.Repeat("长", 60)
	customFilename := longCJK + "." + strings.Repeat("扩", 23)
	for _, tc := range []struct{ title, filename, want string }{
		{"fixture", "", "fixture.mp4"},
		{longASCII, "", longASCII + ".mp4"},
		{longCJK, "", longCJK + ".mp4"},
		{"fixture", customFilename, customFilename},
	} {
		extractor.Register(dashFixtureExtractor{media.URL + "/video.mp4", media.URL + "/audio.m4a", tc.title}, "dash.fixture.test")
		for attempt := 0; attempt < 2; attempt++ {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/download", nil).WithContext(ctx)
			srv.downloadAndStream(c, "https://dash.fixture.test/watch", tc.filename)
			if recorder.Code != http.StatusOK {
				t.Fatalf("return_file: %d %s", recorder.Code, recorder.Body.String())
			}
			_, params, err := mime.ParseMediaType(recorder.Header().Get("Content-Disposition"))
			if err != nil || params["filename"] != tc.want {
				t.Fatalf("attachment filename lost its title or extension: %q (%v)", params["filename"], err)
			}
			responsePath := filepath.Join(t.TempDir(), "response.mp4")
			if err := os.WriteFile(responsePath, recorder.Body.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			streams, err := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-show_entries", "stream=codec_type", "-of", "csv=p=0", responsePath).CombinedOutput()
			if err != nil || !strings.Contains(string(streams), "video") || !strings.Contains(string(streams), "audio") {
				t.Fatalf("response must contain video and audio: %v, %s", err, streams)
			}
		}
	}
	entries, err := os.ReadDir(outputDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("return_file retained files in the permanent output directory: %v, %v", entries, err)
	}
}

func TestDASHMergeFailureReturnsError(t *testing.T) {
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("invalid media"))
	}))
	defer media.Close()
	format := &extractor.VideoFormat{URL: media.URL + "/video", AudioURL: media.URL + "/audio", Ext: "mp4"}
	err := (&Server{}).downloadVideoWithAudio(context.Background(), format, filepath.Join(t.TempDir(), "output.mp4"), nil)
	if err == nil {
		t.Fatal("missing ffmpeg or a failed merge must fail the download")
	}
}
