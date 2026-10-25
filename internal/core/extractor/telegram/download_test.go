package telegram

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
)

type mediaClient struct {
	downloader.Client
	data []byte
	err  error
}

func (c mediaClient) UploadGetFile(ctx context.Context, req *tg.UploadGetFileRequest) (tg.UploadFileClass, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.err != nil {
		return nil, c.err
	}
	start := min(int(req.Offset), len(c.data))
	end := min(start+req.Limit, len(c.data))
	return &tg.UploadFile{Bytes: c.data[start:end]}, nil
}

type parallelMediaClient struct {
	mediaClient
	started      chan int64
	release      <-chan struct{}
	firstWritten <-chan struct{}
	partSize     int64
}

func (c parallelMediaClient) UploadGetFile(ctx context.Context, req *tg.UploadGetFileRequest) (tg.UploadFileClass, error) {
	if req.Offset < 4*c.partSize {
		select {
		case c.started <- req.Offset:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		select {
		case <-c.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	// Force a later block to reach disk before the block at offset zero.
	if req.Offset == 0 {
		select {
		case <-c.firstWritten:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return c.mediaClient.UploadGetFile(ctx, req)
}

func TestMediaDownloadsFourBlocksInParallel(t *testing.T) {
	const partSize = 4096
	data := make([]byte, 6*partSize+317)
	for i := range data {
		data[i] = byte((i*31 + i/partSize) % 251)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	started := make(chan int64, 4)
	release := make(chan struct{})
	firstWritten := make(chan struct{})
	client := parallelMediaClient{mediaClient{data: data}, started, release, firstWritten, partSize}
	path := filepath.Join(t.TempDir(), "parallel.bin")
	var once sync.Once
	var previous int64
	opts := DownloadOptions{ProgressFn: func(downloaded, total int64) {
		if downloaded < previous || downloaded > total {
			t.Errorf("invalid progress: %d after %d, total %d", downloaded, previous, total)
		}
		previous = downloaded
		if downloaded > 0 {
			once.Do(func() { close(firstWritten) })
		}
	}}
	done := make(chan error, 1)
	go func() {
		done <- streamMedia(ctx, client, downloader.NewDownloader().WithPartSize(partSize), &tg.InputDocumentFileLocation{}, path, int64(len(data)), opts)
	}()
	for i := 0; i < 4; i++ {
		select {
		case <-started:
		case <-ctx.Done():
			err := <-done
			t.Fatalf("only %d blocks requested concurrently: %v", i, err)
		}
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("out-of-order blocks corrupted the output: %v", err)
	}
	if previous != int64(len(data)) {
		t.Fatalf("final progress: %d, want %d", previous, len(data))
	}
}

func TestStreamMedia(t *testing.T) {
	data := bytes.Repeat([]byte("telegram media\n"), 100000)
	for _, mode := range []string{"success", "cancel", "network error", "incomplete", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			path := filepath.Join(t.TempDir(), "video.mp4")
			client := mediaClient{data: data}
			if mode == "network error" {
				client.err = errors.New("connection lost")
			}
			size := int64(len(data))
			if mode == "incomplete" {
				size++
			}
			var started string
			var progress [][2]int64
			opts := DownloadOptions{
				OnStart: func(name string) { started = name },
				ProgressFn: func(downloaded, total int64) {
					progress = append(progress, [2]int64{downloaded, total})
					if mode == "cancel" && downloaded > 0 {
						cancel()
					}
					if mode == "truncated" && downloaded == total {
						if err := os.Truncate(path, 1); err != nil {
							t.Errorf("could not simulate truncated output: %v", err)
						}
					}
				},
			}
			err := streamMedia(ctx, client, downloader.NewDownloader(), &tg.InputDocumentFileLocation{}, path, size, opts)
			if started != path || len(progress) == 0 || progress[0] != [2]int64{0, size} {
				t.Fatal("filename and total size were not reported before downloading")
			}
			if mode == "success" {
				if err != nil {
					t.Fatal(err)
				}
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, data) {
					t.Fatalf("downloaded bytes differ: %v", err)
				}
				if progress[len(progress)-1] != [2]int64{size, size} {
					t.Fatal("progress did not reach the complete file size")
				}
				return
			}
			if err == nil {
				t.Fatal("expected a download error")
			}
			if mode == "cancel" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation was lost: %v", err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("partial file was not cleaned up: %v", err)
			}
		})
	}
}

func TestStreamMediaPreservesExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.mp4")
	if err := os.WriteFile(path, []byte("existing download"), 0600); err != nil {
		t.Fatal(err)
	}
	err := streamMedia(context.Background(), nil, nil, nil, path, 0, DownloadOptions{})
	if !errors.Is(err, os.ErrExist) {
		t.Fatalf("expected a file collision, got %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "existing download" {
		t.Fatalf("existing file was modified: %v", err)
	}
}

func TestOutputPathStaysInsideDownloadDirectory(t *testing.T) {
	dir := t.TempDir()
	for _, tc := range []struct{ filename, ext string }{
		{"../../video", "mp4"},
		{"video", "mp4/../../../outside"},
		{"video.mp4", "mp4"},
	} {
		path, err := (DownloadOptions{OutputDir: dir, Filename: tc.filename}).outputPath("default", tc.ext)
		if err != nil {
			t.Fatal(err)
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.Dir(path) != dir {
			t.Fatalf("output escaped the download directory: %q (%v)", path, err)
		}
	}
}
