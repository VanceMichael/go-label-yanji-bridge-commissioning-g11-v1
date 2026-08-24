package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
)

func (d *DB) WithinTx(ctx context.Context, fn func(repository.MutationRepository) error) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	tx, err := d.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	repo := &txRepo{DB: d, q: tx}
	if err := fn(repo); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return errors.Join(err, fmt.Errorf("rollback transaction: %w", rollbackErr))
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}
	return nil
}
