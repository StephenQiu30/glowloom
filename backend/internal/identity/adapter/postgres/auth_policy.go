package postgres

import "context"

// ReadRegistrationEnabled reads the existing settings without changing them.
func (s *Store) ReadRegistrationEnabled(ctx context.Context) (bool, error) {
	var row struct{ Enabled bool }
	result := s.db.WithContext(ctx).Raw(`SELECT registration_enabled AS enabled FROM identity.platform_settings WHERE singleton=true`).Scan(&row)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1 && row.Enabled, nil
}
