package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/domain"
	"github.com/VanceMichael/go-label-yanji-bridge-commissioning-g11-v1/internal/repository"
)

func (d *DB) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	return appendAudit(ctx, d.queryer(), e)
}
func (t *txRepo) AppendAudit(ctx context.Context, e domain.AuditEvent) error {
	return appendAudit(ctx, t.queryer(), e)
}
func appendAudit(ctx context.Context, q querier, e domain.AuditEvent) error {
	_, err := q.ExecContext(ctx, `INSERT INTO audit_events(id,organization_id,actor_id,request_id,object_type,object_id,action,result,detail,created_at) VALUES(?,?,?,?,?,?,?,?,?,?)`, e.ID, e.Organization, e.ActorID, e.RequestID, e.ObjectType, e.ObjectID, e.Action, e.Result, e.Detail, timestamp(e.CreatedAt))
	if err != nil {
		return fmt.Errorf("append audit: %w", err)
	}
	return nil
}
func (d *DB) GetIdempotency(ctx context.Context, s repository.IdempotencyScope) (repository.IdempotencyRecord, error) {
	return getIdempotency(ctx, d.queryer(), s)
}
func (t *txRepo) GetIdempotency(ctx context.Context, s repository.IdempotencyScope) (repository.IdempotencyRecord, error) {
	return getIdempotency(ctx, t.queryer(), s)
}
func getIdempotency(ctx context.Context, q querier, s repository.IdempotencyScope) (repository.IdempotencyRecord, error) {
	var r repository.IdempotencyRecord
	var created, expires string
	err := q.QueryRowContext(ctx, `SELECT request_hash,status_code,response,created_at,expires_at FROM idempotency_keys WHERE organization_id=? AND method=? AND path=? AND key=?`, s.Organization, s.Method, s.Path, s.Key).Scan(&r.RequestHash, &r.StatusCode, &r.Response, &created, &expires)
	if err != nil {
		return r, mapNotFound(err)
	}
	r.Scope = s
	var parseErr error
	if r.CreatedAt, parseErr = parseTime(created); parseErr != nil {
		return r, parseErr
	}
	if r.ExpiresAt, parseErr = parseTime(expires); parseErr != nil {
		return r, parseErr
	}
	return r, nil
}
func (d *DB) PutIdempotency(ctx context.Context, r repository.IdempotencyRecord) error {
	return putIdempotency(ctx, d.queryer(), r)
}
func (t *txRepo) PutIdempotency(ctx context.Context, r repository.IdempotencyRecord) error {
	return putIdempotency(ctx, t.queryer(), r)
}
func putIdempotency(ctx context.Context, q querier, r repository.IdempotencyRecord) error {
	_, err := q.ExecContext(ctx, `INSERT INTO idempotency_keys(organization_id,method,path,key,request_hash,status_code,response,created_at,expires_at) VALUES(?,?,?,?,?,?,?,?,?)`, r.Scope.Organization, r.Scope.Method, r.Scope.Path, r.Scope.Key, r.RequestHash, r.StatusCode, r.Response, timestamp(r.CreatedAt), timestamp(r.ExpiresAt))
	if err != nil {
		return fmt.Errorf("store idempotency response: %w", err)
	}
	return nil
}
func (d *DB) EnqueueJob(ctx context.Context, j domain.Job) error {
	return enqueueJob(ctx, d.queryer(), j)
}
func (t *txRepo) EnqueueJob(ctx context.Context, j domain.Job) error {
	return enqueueJob(ctx, t.queryer(), j)
}
func enqueueJob(ctx context.Context, q querier, j domain.Job) error {
	_, err := q.ExecContext(ctx, `INSERT INTO jobs(id,kind,payload,status,attempts,max_attempts,available_at,lease_owner,lease_until,last_error,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, j.ID, j.Kind, j.Payload, j.Status, j.Attempts, j.MaxAttempts, timestamp(j.AvailableAt), j.LeaseOwner, timestamp(j.LeaseUntil), j.LastError, timestamp(j.CreatedAt))
	if err != nil {
		return fmt.Errorf("enqueue job: %w", err)
	}
	return nil
}
func (d *DB) ClaimJob(ctx context.Context, owner string, now time.Time, lease time.Duration) (domain.Job, error) {
	return claimJob(ctx, d.db, owner, now, lease)
}
func (t *txRepo) ClaimJob(ctx context.Context, owner string, now time.Time, lease time.Duration) (domain.Job, error) {
	return claimJob(ctx, t.q, owner, now, lease)
}
func claimJob(ctx context.Context, q querier, owner string, now time.Time, lease time.Duration) (domain.Job, error) {
	row := q.QueryRowContext(ctx, `UPDATE jobs SET status='leased',lease_owner=?,lease_until=?,attempts=attempts+1 WHERE id=(SELECT id FROM jobs WHERE available_at<=? AND (status='pending' OR (status='leased' AND lease_until<=?)) ORDER BY available_at,id LIMIT 1) RETURNING id,kind,payload,status,attempts,max_attempts,available_at,COALESCE(lease_owner,''),COALESCE(lease_until,''),COALESCE(last_error,''),created_at`, owner, timestamp(now.Add(lease)), timestamp(now), timestamp(now))
	var j domain.Job
	var available, leaseUntil, created string
	if err := row.Scan(&j.ID, &j.Kind, &j.Payload, &j.Status, &j.Attempts, &j.MaxAttempts, &available, &j.LeaseOwner, &leaseUntil, &j.LastError, &created); err != nil {
		return j, mapNotFound(err)
	}
	var err error
	if j.AvailableAt, err = parseTime(available); err != nil {
		return j, err
	}
	if j.LeaseUntil, err = parseTime(leaseUntil); err != nil {
		return j, err
	}
	if j.CreatedAt, err = parseTime(created); err != nil {
		return j, err
	}
	return j, nil
}
func (d *DB) CompleteJob(ctx context.Context, id, owner string, now time.Time) error {
	return completeJob(ctx, d.queryer(), id, owner, now)
}
func (t *txRepo) CompleteJob(ctx context.Context, id, owner string, now time.Time) error {
	return completeJob(ctx, t.queryer(), id, owner, now)
}
func completeJob(ctx context.Context, q querier, id, owner string, now time.Time) error {
	released, err := q.ExecContext(ctx, `UPDATE jobs SET lease_owner=NULL,lease_until=NULL WHERE id=? AND status='leased' AND lease_owner=? AND lease_until>?`, id, owner, timestamp(now))
	if err != nil {
		return fmt.Errorf("release completed job lease: %w", err)
	}
	if err := requireAffected(released, "job", id, 0); err != nil {
		return err
	}
	completed, err := q.ExecContext(ctx, `UPDATE jobs SET status='completed',last_error='' WHERE id=? AND status='leased' AND lease_owner IS NULL`, id)
	if err != nil {
		return fmt.Errorf("mark job completed: %w", err)
	}
	return requireAffected(completed, "job", id, 0)
}
func (d *DB) RetryJob(ctx context.Context, j domain.Job, message string, at time.Time) error {
	return retryJob(ctx, d.queryer(), j, message, at)
}
func (t *txRepo) RetryJob(ctx context.Context, j domain.Job, message string, at time.Time) error {
	return retryJob(ctx, t.queryer(), j, message, at)
}
func retryJob(ctx context.Context, q querier, j domain.Job, message string, at time.Time) error {
	status := domain.JobPending
	if !j.Retryable() {
		status = domain.JobFailed
	}
	result, err := q.ExecContext(ctx, `UPDATE jobs SET status=?,available_at=?,lease_owner=NULL,lease_until=NULL,last_error=? WHERE id=? AND status='leased' AND lease_owner=?`, status, timestamp(at), message, j.ID, j.LeaseOwner)
	if err != nil {
		return fmt.Errorf("retry job: %w", err)
	}
	return requireAffected(result, "job", j.ID, 0)
}
func (d *DB) ReleaseLeases(ctx context.Context, owner string, now time.Time) error {
	return releaseLeases(ctx, d.queryer(), owner, now)
}
func (t *txRepo) ReleaseLeases(ctx context.Context, owner string, now time.Time) error {
	return releaseLeases(ctx, t.queryer(), owner, now)
}
func releaseLeases(ctx context.Context, q querier, owner string, now time.Time) error {
	_, err := q.ExecContext(ctx, `UPDATE jobs SET status='pending',available_at=?,lease_owner=NULL,lease_until=NULL WHERE status='leased' AND lease_owner=?`, timestamp(now), owner)
	return err
}

var _ = errors.Is
var _ = sql.ErrNoRows
