package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
)

func timestamp(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func optionalText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse database timestamp: %w", err)
	}
	return parsed, nil
}

func mapNotFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNotFound
	}
	return err
}

func requireAffected(result sql.Result, entity, id string, version int64) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read %s update result: %w", entity, err)
	}
	if count == 0 {
		return newVersionConflict(entity, id, version)
	}
	return nil
}

func newVersionConflict(entity, id string, version int64) error {
	return &domain.VersionConflict{Entity: entity, ID: id, Version: version}
}

type scanner interface{ Scan(...any) error }

func nullable(value sql.NullString) string {
	if value.Valid {
		return value.String
	}
	return ""
}
