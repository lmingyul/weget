package telegram

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/gotd/td/session"
	"github.com/guiyumin/vget/internal/core/config"
)

// SessionFile uses the same session imported by the installed vget CLI.
func SessionFile() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "telegram", "desktop-session.json"), nil
}

func loadSession(ctx context.Context, path string) (*session.StorageMemory, error) {
	data, err := (&session.FileStorage{Path: path}).LoadSession(ctx)
	if err != nil {
		return nil, fmt.Errorf("cannot read Telegram session; run 'vget telegram login --import-desktop' in the terminal first: %w", err)
	}
	// Each download updates its own copy; the CLI's session stays untouched.
	storage := new(session.StorageMemory)
	if err := storage.StoreSession(ctx, data); err != nil {
		return nil, err
	}
	return storage, nil
}
