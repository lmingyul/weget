package extractor

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestBilibiliExtractorRegistered(t *testing.T) {
	for _, rawURL := range []string{
		"https://www.bilibili.com/video/BV1vVnA6XEEa/",
		"https://bilibili.com/video/av117473697333496",
		"https://b23.tv/example123",
	} {
		ext := Match(rawURL)
		if ext == nil || ext.Name() != "bilibili" {
			t.Errorf("no Bilibili extractor for %s", rawURL)
		}
	}
}

func TestBilibiliVideoIDConversion(t *testing.T) {
	aid, err := BVToAV("BV1vVnA6XEEa")
	if err != nil || aid != 117473697333496 {
		t.Fatalf("wrong AV ID: %d (%v)", aid, err)
	}
	bvid, err := AVToBV(aid)
	if err != nil || bvid != "BV1vVnA6XEEa" {
		t.Fatalf("wrong BV ID: %q (%v)", bvid, err)
	}
}

type bilibiliTransport func(*http.Request) (*http.Response, error)

func (f bilibiliTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestBilibiliExtraction(t *testing.T) {
	const videoURL = "https://www.bilibili.com/video/BV1vVnA6XEEa/"
	const shortURL = "https://b23.tv/example123"
	defaultFilenames := make(map[string]string)
	for _, tc := range []struct {
		name, rawURL, redirectURL, wantCID string
		singlePart                         bool
	}{
		{"direct", videoURL, "", "123", false},
		{"short", shortURL, videoURL, "123", false},
		{"without scheme", "bilibili.com/video/BV1vVnA6XEEa/", "", "123", false},
		{"short without scheme", "b23.tv/example123", videoURL, "123", false},
		{"first page", videoURL + "?p=1", "", "123", false},
		{"second page", videoURL + "?p=2", "", "456", false},
		{"AV second page", "https://bilibili.com/video/av117473697333496?p=2", "", "456", false},
		{"short second page", shortURL, videoURL + "?p=2", "456", false},
		{"single part", videoURL, "", "123", true},
		{"single part explicit page", videoURL + "?p=1", "", "123", true},
		{"page zero", videoURL + "?p=0", "", "", false},
		{"negative page", videoURL + "?p=-1", "", "", false},
		{"out of range", videoURL + "?p=3", "", "", false},
		{"non-numeric page", videoURL + "?p=abc", "", "", false},
		{"empty page", videoURL + "?p=", "", "", false},
		{"malformed page escape", videoURL + "?p=%zz", "", "", false},
		{"page overflow", videoURL + "?p=999999999999999999999999", "", "", false},
		{"short out of range", shortURL, videoURL + "?p=3", "", false},
		{"short non-numeric page", shortURL, videoURL + "?p=abc", "", false},
		{"short malformed page escape", shortURL, videoURL + "?p=%zz", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ext := &BilibiliExtractor{cookie: "SESSDATA=test-fixture"}
			ext.client = &http.Client{
				CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
				Transport: bilibiliTransport(func(req *http.Request) (*http.Response, error) {
					body := ""
					header := make(http.Header)
					status := http.StatusOK
					if req.URL.Host == "api.bilibili.com" {
						if req.Header.Get("Cookie") != "SESSDATA=test-fixture" || req.Header.Get("Referer") != "https://www.bilibili.com/" {
							t.Error("missing API authentication or Referer header")
						}
					}
					switch req.URL.Path {
					case "/example123":
						if req.Header.Get("Cookie") != "" {
							t.Error("API cookie sent to short-link resolver")
						}
						status = http.StatusFound
						header.Set("Location", tc.redirectURL)
					case "/x/web-interface/nav":
						body = `{"code":-101,"data":{"wbi_img":{"img_url":"https://i0.hdslb.com/bfs/wbi/0123456789abcdef0123456789abcdef.png","sub_url":"https://i0.hdslb.com/bfs/wbi/fedcba9876543210fedcba9876543210.png"}}}`
					case "/x/web-interface/view":
						if req.URL.Query().Get("aid") != "117473697333496" {
							t.Error("metadata request used the wrong video ID")
						}
						body = `{"code":0,"data":{"title":"Local fixture","duration":12,"owner":{"name":"Fixture"},"pages":[{"cid":123,"page":1},{"cid":456,"page":2}]}}`
						if tc.singlePart {
							body = `{"code":0,"data":{"title":"Local fixture","duration":12,"owner":{"name":"Fixture"},"pages":[{"cid":123,"page":1}]}}`
						}
					case "/x/player/wbi/playurl":
						if tc.wantCID == "" {
							t.Fatal("play URL requested for an invalid page")
						}
						q := req.URL.Query()
						if q.Get("cid") != tc.wantCID {
							t.Errorf("play URL CID = %q, want %q", q.Get("cid"), tc.wantCID)
						}
						if len(q.Get("w_rid")) != 32 || q.Get("wts") == "" {
							t.Error("play URL request was not signed")
						}
						body = `{"code":0,"data":{"dash":{"video":[{"id":80,"baseUrl":"https://media.example/video","bandwidth":3000000,"width":1920,"height":1080,"codecid":7}],"audio":[{"baseUrl":"https://media.example/audio-low","bandwidth":64000},{"baseUrl":"https://media.example/audio-high","bandwidth":192000}]}}}`
					default:
						t.Errorf("unexpected API request: %s", req.URL.Path)
					}
					return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
				}),
			}
			media, err := ext.extract(tc.rawURL)
			if tc.wantCID == "" {
				if err == nil || !strings.Contains(err.Error(), "page") {
					t.Fatalf("expected a page validation error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			video, ok := media.(*VideoMedia)
			wantTitle := "P1 - Local fixture"
			if tc.wantCID == "456" {
				wantTitle = "P2 - Local fixture"
			}
			if tc.singlePart {
				wantTitle = "Local fixture"
			}
			if !ok || video.ID != "BV1vVnA6XEEa" || video.Title != wantTitle || len(video.Formats) != 1 {
				t.Fatalf("unexpected video metadata: %+v", media)
			}
			format := video.Formats[0]
			if format.URL != "https://media.example/video" || format.AudioURL != "https://media.example/audio-high" || format.Height != 1080 {
				t.Fatalf("wrong DASH streams: %+v", format)
			}
			if format.Headers["Referer"] != "https://www.bilibili.com/" || format.Headers["User-Agent"] == "" {
				t.Fatal("missing CDN download headers")
			}
			if !tc.singlePart {
				defaultFilenames[tc.wantCID] = SanitizeFilename(video.Title) + "." + format.Ext
			}
		})
	}
	if defaultFilenames["123"] == "" || defaultFilenames["456"] == "" || defaultFilenames["123"] == defaultFilenames["456"] {
		t.Fatalf("multipart default filenames must differ: %v", defaultFilenames)
	}
}

func TestBilibiliQualityLabels(t *testing.T) {
	var streams BilibiliStreamInfo
	if err := json.Unmarshal([]byte(`{"video":[{"id":80,"baseUrl":"https://media.example/1080","height":1080,"codecid":7},{"id":64,"baseUrl":"https://media.example/720","height":720,"codecid":7}]}`), &streams); err != nil {
		t.Fatal(err)
	}
	formats := (&BilibiliExtractor{}).buildFormats(&streams)
	if len(formats) != 2 {
		t.Fatalf("got %d formats, want 2", len(formats))
	}
	for i, quality := range []string{"1080p", "720p"} {
		if !strings.Contains(formats[i].Quality, quality) {
			t.Errorf("documented CLI quality %q does not match label %q", quality, formats[i].Quality)
		}
	}
}

func TestBilibiliRejectsUnsupportedURLs(t *testing.T) {
	ext := &BilibiliExtractor{}
	for _, rawURL := range []string{
		"https://example.com/bilibili.com/video/BV1vVnA6XEEa/",
		"https://www.bilibili.com.evil.example/video/BV1vVnA6XEEa/",
		"https://www.bilibili.com@evil.example/video/BV1vVnA6XEEa/",
		"https://www.bilibili.com/video/BVbad/",
		"https://www.bilibili.com/bangumi/play/ep123",
	} {
		u, err := url.Parse(rawURL)
		if err != nil {
			t.Fatal(err)
		}
		if ext.Match(u) {
			t.Errorf("matched unsupported URL %s", rawURL)
		}
	}
}
