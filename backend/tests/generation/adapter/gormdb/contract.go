package gormdb

import "gorm.io/gorm"

// Database keeps Generation acceptance fixtures behind the test GORM adapter boundary.
type Database = gorm.DB

// DeleteWithoutHooks removes only test-owned immutable facts during fixture cleanup.
func DeleteWithoutHooks(database *Database, value any, query string, arguments ...any) error {
	return database.Session(&gorm.Session{SkipHooks: true}).Where(query, arguments...).Delete(value).Error
}
