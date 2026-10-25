package telegram

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/dcs"
	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
)

type telegramInvoker func(context.Context, bin.Encoder, bin.Decoder) error

func (f telegramInvoker) Invoke(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
	return f(ctx, input, output)
}

func TestDownloadClientCancellation(t *testing.T) {
	for _, cancelOriginal := range []bool{true, false} {
		t.Run(fmt.Sprintf("cancel_original=%t", cancelOriginal), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			dials := 0
			client := telegram.NewClient(DesktopAppID, DesktopAppHash, telegram.Options{
				NoUpdates: true,
				DCList: dcs.List{Options: []tg.DCOption{
					{ID: 2, IPAddress: "127.0.0.1", Port: 1},
				}},
				Resolver: dcs.Plain(dcs.PlainOptions{
					Dial: func(context.Context, string, string) (net.Conn, error) {
						dials++
						if cancelOriginal {
							cancel()
						}
						return nil, context.Canceled
					},
				}),
				ReconnectionBackoff: func() backoff.BackOff { return &backoff.StopBackOff{} },
			})
			result, err := downloadWithClient(ctx, client, &Message{}, DownloadOptions{})
			if dials != 1 {
				t.Fatalf("expected one offline connection attempt, got %d", dials)
			}
			if cancelOriginal {
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("original cancellation was lost: result=%v, err=%v", result, err)
				}
			} else {
				if ctx.Err() != nil {
					t.Fatalf("original context was unexpectedly cancelled: %v", ctx.Err())
				}
				if err == nil {
					t.Fatal("client stopped without producing media but download reported success")
				}
			}
			if result != nil {
				t.Fatalf("failed download returned a result: %+v", result)
			}
		})
	}
}

func TestChannelPaginationFollowsServiceMessage(t *testing.T) {
	for _, endpoint := range []struct {
		name   string
		peer   tg.PeerClass
		cursor tg.InputPeerClass
	}{
		{"user", &tg.PeerUser{UserID: 100}, &tg.InputPeerUser{UserID: 100, AccessHash: 1100}},
		{"group", &tg.PeerChat{ChatID: 200}, &tg.InputPeerChat{ChatID: 200}},
		{"channel", &tg.PeerChannel{ChannelID: 300}, &tg.InputPeerChannel{ChannelID: 300, AccessHash: 1300}},
	} {
		t.Run(endpoint.name, func(t *testing.T) {
			first := &tg.MessagesDialogsSlice{Count: 101}
			for i := 1; i <= 100; i++ {
				peer := &tg.PeerUser{UserID: int64(i)}
				first.Dialogs = append(first.Dialogs, &tg.Dialog{Peer: peer, TopMessage: i})
				first.Messages = append(first.Messages, &tg.Message{ID: i, Date: 2000 + i, PeerID: peer})
				first.Users = append(first.Users, &tg.User{ID: int64(i), AccessHash: int64(1000 + i)})
			}
			first.Dialogs[99] = &tg.Dialog{Peer: endpoint.peer, TopMessage: 777}
			first.Messages[99] = &tg.MessageService{ID: 777, Date: 1234, PeerID: endpoint.peer}
			// Entity and message order need not match the order of dialogs.
			first.Messages[98], first.Messages[99] = first.Messages[99], first.Messages[98]
			first.Chats = []tg.ChatClass{
				&tg.Channel{ID: 300, AccessHash: 1300},
				&tg.Chat{ID: 200},
				&tg.Channel{ID: 999, AccessHash: 1999},
			}
			calls := 0
			api := tg.NewClient(telegramInvoker(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
				if err := ctx.Err(); err != nil {
					return err
				}
				req, ok := input.(*tg.MessagesGetDialogsRequest)
				if !ok {
					return fmt.Errorf("unexpected RPC: %T", input)
				}
				calls++
				switch calls {
				case 1:
					output.(*tg.MessagesDialogsBox).Dialogs = first
				case 2:
					if req.OffsetID != 777 || req.OffsetDate != 1234 || !reflect.DeepEqual(req.OffsetPeer, endpoint.cursor) {
						t.Errorf("wrong next-page cursor: id=%d date=%d peer=%+v", req.OffsetID, req.OffsetDate, req.OffsetPeer)
					}
					peer := &tg.PeerChannel{ChannelID: 400}
					output.(*tg.MessagesDialogsBox).Dialogs = &tg.MessagesDialogsSlice{
						Count:    101,
						Dialogs:  []tg.DialogClass{&tg.Dialog{Peer: peer, TopMessage: 1}},
						Messages: []tg.MessageClass{&tg.Message{ID: 1, Date: 1000, PeerID: peer}},
						Chats:    []tg.ChatClass{&tg.Channel{ID: 400, AccessHash: 1400, Title: "Target"}},
					}
				case 3:
					if req.OffsetID != 1 || req.OffsetDate != 1000 || !reflect.DeepEqual(req.OffsetPeer, &tg.InputPeerChannel{ChannelID: 400, AccessHash: 1400}) {
						t.Errorf("wrong terminal-page cursor: %+v", req)
					}
					output.(*tg.MessagesDialogsBox).Dialogs = &tg.MessagesDialogs{}
				default:
					return fmt.Errorf("unexpected extra dialog page")
				}
				return nil
			}))
			channel, err := resolvePrivateChannel(context.Background(), api, 400)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 3 || channel.ID != 400 || channel.AccessHash != 1400 {
				t.Fatalf("wrong channel after pagination: %+v (%d calls)", channel, calls)
			}
		})
	}
}

func TestCaptionFallbackFilenames(t *testing.T) {
	for _, tc := range []struct{ name, caption, title string }{
		{"URL only", "https://example.com/post", "telegram_42"},
		{"long URL", "https://example.com/" + strings.Repeat("a", 200), "telegram_42"},
		{"caption with URL", "Useful caption https://example.com/post", "Useful caption"},
	} {
		for _, kind := range []string{"photo", "document"} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				data := []byte("offline media fixture")
				api := tg.NewClient(telegramInvoker(func(ctx context.Context, input bin.Encoder, output bin.Decoder) error {
					req, ok := input.(*tg.UploadGetFileRequest)
					if !ok {
						return fmt.Errorf("unexpected RPC: %T", input)
					}
					file, err := (mediaClient{data: data}).UploadGetFile(ctx, req)
					if err == nil {
						output.(*tg.UploadFileBox).File = file
					}
					return err
				}))
				opts := DownloadOptions{OutputDir: t.TempDir()}
				msg := &tg.Message{ID: 42, Message: tc.caption}
				var result *DownloadResult
				var err error
				ext := "pdf"
				if kind == "photo" {
					ext = "jpg"
					photo := &tg.Photo{Sizes: []tg.PhotoSizeClass{&tg.PhotoSize{Type: "x", W: 100, H: 100, Size: len(data)}}}
					result, err = downloadPhoto(context.Background(), api, downloader.NewDownloader(), &tg.MessageMediaPhoto{Photo: photo}, msg, opts)
				} else {
					doc := &tg.Document{MimeType: "application/pdf", Size: int64(len(data))}
					result, err = downloadDocument(context.Background(), api, downloader.NewDownloader(), &tg.MessageMediaDocument{Document: doc}, msg, opts)
				}
				if err != nil {
					t.Fatal(err)
				}
				if result.Title != tc.title || filepath.Base(result.Filename) != tc.title+"."+ext {
					t.Fatalf("wrong caption fallback: %+v", result)
				}
				got, err := os.ReadFile(result.Filename)
				if err != nil || !bytes.Equal(got, data) {
					t.Fatalf("offline fixture output differs: %v", err)
				}
			})
		}
	}
}

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
