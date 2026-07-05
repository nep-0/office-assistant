package store

import "context"

func (s *Store) EnsureProviderDefaults(ctx context.Context, defaults map[string]ProviderSetting) error {
	for purpose, setting := range defaults {
		_, err := s.db.ExecContext(ctx, `
INSERT INTO provider_settings (purpose, base_url, model, api_key)
VALUES (?, ?, ?, ?)
ON CONFLICT(purpose) DO NOTHING
`, purpose, setting.BaseURL, setting.Model, setting.APIKey)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListProviderSettings(ctx context.Context) ([]ProviderSetting, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT purpose, base_url, model, api_key, updated_at
FROM provider_settings
ORDER BY purpose
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []ProviderSetting
	for rows.Next() {
		setting, err := scanProviderSetting(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	return settings, rows.Err()
}

func (s *Store) FindProviderSetting(ctx context.Context, purpose string) (ProviderSetting, error) {
	return scanProviderSetting(s.db.QueryRowContext(ctx, `
SELECT purpose, base_url, model, api_key, updated_at
FROM provider_settings
WHERE purpose = ?
`, purpose))
}

func (s *Store) UpdateProviderSetting(ctx context.Context, setting ProviderSetting) (ProviderSetting, error) {
	_, err := s.db.ExecContext(ctx, `
UPDATE provider_settings
SET base_url = ?, model = ?, api_key = ?, updated_at = CURRENT_TIMESTAMP
WHERE purpose = ?
`, setting.BaseURL, setting.Model, setting.APIKey, setting.Purpose)
	if err != nil {
		return ProviderSetting{}, err
	}
	return s.FindProviderSetting(ctx, setting.Purpose)
}
