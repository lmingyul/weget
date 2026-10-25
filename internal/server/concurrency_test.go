package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/guiyumin/vget/internal/core/downloader"
)

func TestHTTPDownloadConcurrency(t *testing.T) {
	for _, cancelDownload := range []bool{false, true} {
		t.Run(map[bool]string{false: "complete", true: "cancel"}[cancelDownload], func(t *testing.T) {
			ctx, cancel := context.WithTimeout(downloader.WithConcurrencyTracking(context.Background()), 5*time.Second)
			defer cancel()
			started := make(chan struct{}, 2)
			release := make(chan struct{})
			media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				started <- struct{}{}
				select {
				case <-release:
					_, _ = w.Write([]byte("fixture"))
				case <-r.Context().Done():
				}
			}))
			defer media.Close()
			done := make(chan error, 2)
			for i := 0; i < 2; i++ {
				path := filepath.Join(t.TempDir(), "stream.bin")
				go func() { done <- downloadFile(ctx, media.URL, path, nil, nil) }()
			}
			for i := 0; i < 2; i++ {
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal("parallel requests did not start")
				}
			}
			if count := downloader.ActiveTransfers(ctx); count != 2 {
				t.Errorf("two simultaneous streams report %d transfers", count)
			}
			if cancelDownload {
				cancel()
			} else {
				close(release)
			}
			for i := 0; i < 2; i++ {
				err := <-done
				if (err != nil) != cancelDownload {
					t.Errorf("unexpected transfer result: %v", err)
				}
			}
			if count := downloader.ActiveTransfers(ctx); count != 0 {
				t.Errorf("finished requests left %d transfers", count)
			}
		})
	}
}

func TestJobAPIReportsCurrentConcurrency(t *testing.T) {
	started := make(chan struct{})
	done := make(chan struct{})
	queue := NewJobQueue(1, t.TempDir(), func(ctx context.Context, _, _ string, _ func(int64, int64)) error {
		defer downloader.TrackTransfer(ctx)()
		close(started)
		<-ctx.Done()
		return ctx.Err()
	})
	job, err := queue.AddJob("https://media.example/file", "")
	if err != nil {
		t.Fatal(err)
	}
	go func() { defer close(done); queue.processJob(job) }()
	defer func() {
		job.cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("cancelled job did not stop")
		}
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("job did not start")
	}
	srv := &Server{jobQueue: queue}
	for _, all := range []bool{false, true} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Params = gin.Params{{Key: "id", Value: job.ID}}
		if all {
			srv.handleGetJobs(c)
		} else {
			srv.handleStatus(c)
		}
		var response struct {
			Data struct {
				Concurrency int   `json:"concurrency"`
				Jobs        []Job `json:"jobs"`
			} `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		count := response.Data.Concurrency
		if all {
			if len(response.Data.Jobs) != 1 {
				t.Fatalf("wrong job list: %s", recorder.Body.String())
			}
			count = response.Data.Jobs[0].Concurrency
		}
		if count != 1 {
			t.Errorf("job API reports %d transfers, want 1", count)
		}
	}
	queue.CancelJob(job.ID)
	if queue.GetJob(job.ID).Concurrency != 0 || queue.GetAllJobs()[0].Concurrency != 0 {
		t.Fatal("cancelled jobs must report zero concurrency")
	}
}
