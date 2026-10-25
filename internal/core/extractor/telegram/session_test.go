package telegram

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSessionPreservesCLIFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "desktop-session.json")
	original := []byte(`{"version":1,"data":{}}`)
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	storage, err := loadSession(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := storage.LoadSession(ctx)
	if err != nil || !bytes.Equal(loaded, original) {
		t.Fatalf("did not reuse the saved session: %v", err)
	}
	if err := storage.StoreSession(ctx, []byte("updated in memory")); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(saved, original) {
		t.Fatalf("CLI session was modified: %v", err)
	}
}

func TestMissingSessionExplainsHowToLogin(t *testing.T) {
	_, err := loadSession(context.Background(), filepath.Join(t.TempDir(), "missing.json"))
	if err == nil || !strings.Contains(err.Error(), "vget telegram login --import-desktop") {
		t.Fatalf("missing login instruction: %v", err)
	}
}
