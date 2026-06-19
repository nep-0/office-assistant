package storage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) GetProviderSettings(ctx context.Context) (ProviderSettings, error) {
	settings := ProviderSettings{
		Embedding: ProviderSlot{Kind: "mock", Model: "mock-embedding"},
		Chat:      ProviderSlot{Kind: "mock", Model: "mock-chat"},
	}

	rows, err := s.db.QueryContext(ctx, `select slot, kind, base_url, model, api_key from provider_settings`)
	if err != nil {
		return ProviderSettings{}, fmt.Errorf("query provider settings: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var slot string
		var value ProviderSlot
		if err := rows.Scan(&slot, &value.Kind, &value.BaseURL, &value.Model, &value.APIKey); err != nil {
			return ProviderSettings{}, fmt.Errorf("scan provider settings: %w", err)
		}
		switch slot {
		case "embedding":
			settings.Embedding = value
		case "chat":
			settings.Chat = value
		}
	}
	if err := rows.Err(); err != nil {
		return ProviderSettings{}, fmt.Errorf("iterate provider settings: %w", err)
	}

	return settings, nil
}

func (s *Store) SaveProviderSettings(ctx context.Context, settings ProviderSettings) error {
	if err := validateSlot(settings.Embedding); err != nil {
		return fmt.Errorf("embedding provider: %w", err)
	}
	if err := validateSlot(settings.Chat); err != nil {
		return fmt.Errorf("chat provider: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin provider settings transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for slot, value := range map[string]ProviderSlot{
		"embedding": settings.Embedding,
		"chat":      settings.Chat,
	} {
		_, err := tx.ExecContext(ctx, `insert into provider_settings (slot, kind, base_url, model, api_key, updated_at)
			values (?, ?, ?, ?, ?, ?)
			on conflict(slot) do update set
				kind = excluded.kind,
				base_url = excluded.base_url,
				model = excluded.model,
				api_key = excluded.api_key,
				updated_at = excluded.updated_at`,
			slot, value.Kind, value.BaseURL, value.Model, value.APIKey, now)
		if err != nil {
			return fmt.Errorf("save %s provider settings: %w", slot, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit provider settings transaction: %w", err)
	}
	return nil
}

func Masked(settings ProviderSettings) ProviderSettings {
	settings.Embedding.APIKey = mask(settings.Embedding.APIKey)
	settings.Chat.APIKey = mask(settings.Chat.APIKey)
	return settings
}

func mask(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "********"
}

func validateSlot(slot ProviderSlot) error {
	if strings.TrimSpace(slot.Kind) == "" {
		return errors.New("kind is required")
	}
	if strings.TrimSpace(slot.Kind) == "mock" {
		return nil
	}
	if strings.TrimSpace(slot.BaseURL) == "" {
		return errors.New("base_url is required")
	}
	if strings.TrimSpace(slot.Model) == "" {
		return errors.New("model is required")
	}
	return nil
}
