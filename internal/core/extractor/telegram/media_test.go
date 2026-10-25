package telegram

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestFindLargestPhotoSizeProgressive(t *testing.T) {
	progressive := &tg.PhotoSizeProgressive{Type: "y", W: 1280, H: 720, Sizes: []int{500, 1200, 2400}}
	for _, tc := range []struct {
		name  string
		sizes []tg.PhotoSizeClass
		want  tg.PhotoSize
	}{
		{"progressive only", []tg.PhotoSizeClass{progressive}, tg.PhotoSize{Type: "y", W: 1280, H: 720, Size: 2400}},
		{"progressive largest", []tg.PhotoSizeClass{&tg.PhotoSize{Type: "s", W: 100, H: 100, Size: 100}, progressive}, tg.PhotoSize{Type: "y", W: 1280, H: 720, Size: 2400}},
		{"regular largest", []tg.PhotoSizeClass{progressive, &tg.PhotoSize{Type: "w", W: 1920, H: 1080, Size: 4000}}, tg.PhotoSize{Type: "w", W: 1920, H: 1080, Size: 4000}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := FindLargestPhotoSize(tc.sizes)
			if got == nil || *got != tc.want {
				t.Fatalf("wrong largest photo size: %+v, want %+v", got, tc.want)
			}
		})
	}
}
