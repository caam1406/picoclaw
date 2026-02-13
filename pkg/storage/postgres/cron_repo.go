package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/storage/repository"
)

type cronRepository struct {
	db dbExecutor
}

// NewCronRepository creates a new PostgreSQL cron repository.
func NewCronRepository(db dbExecutor) repository.CronRepository {
	return &cronRepository{db: db}
}

func (r *cronRepository) GetJob(ctx context.Context, jobID string) (*repository.CronJob, error) {
	query := `SELECT id, name, enabled, schedule, payload, state, created_at_ms, updated_at_ms, delete_after_run
	          FROM cron_jobs
	          WHERE id = $1`

	var job repository.CronJob
	var scheduleJSON, payloadJSON, stateJSON []byte

	err := r.db.QueryRowContext(ctx, query, jobID).Scan(
		&job.ID,
		&job.Name,
		&job.Enabled,
		&scheduleJSON,
		&payloadJSON,
		&stateJSON,
		&job.CreatedAtMS,
		&job.UpdatedAtMS,
		&job.DeleteAfterRun,
	)

	if err != nil {
		return nil, err
	}

	// Unmarshal JSONB fields
	if err := json.Unmarshal(scheduleJSON, &job.Schedule); err != nil {
		return nil, fmt.Errorf("failed to unmarshal schedule: %w", err)
	}
	if err := json.Unmarshal(payloadJSON, &job.Payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	if err := json.Unmarshal(stateJSON, &job.State); err != nil {
		return nil, fmt.Errorf("failed to unmarshal state: %w", err)
	}

	return &job, nil
}

func (r *cronRepository) ListJobs(ctx context.Context, includeDisabled bool) ([]repository.CronJob, error) {
	var query string
	if includeDisabled {
		query = `SELECT id, name, enabled, schedule, payload, state, created_at_ms, updated_at_ms, delete_after_run
		         FROM cron_jobs
		         ORDER BY updated_at_ms DESC`
	} else {
		query = `SELECT id, name, enabled, schedule, payload, state, created_at_ms, updated_at_ms, delete_after_run
		         FROM cron_jobs
		         WHERE enabled = true
		         ORDER BY updated_at_ms DESC`
	}

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []repository.CronJob
	for rows.Next() {
		var job repository.CronJob
		var scheduleJSON, payloadJSON, stateJSON []byte

		if err := rows.Scan(
			&job.ID,
			&job.Name,
			&job.Enabled,
			&scheduleJSON,
			&payloadJSON,
			&stateJSON,
			&job.CreatedAtMS,
			&job.UpdatedAtMS,
			&job.DeleteAfterRun,
		); err != nil {
			return nil, err
		}

		// Unmarshal JSONB fields
		if err := json.Unmarshal(scheduleJSON, &job.Schedule); err != nil {
			return nil, fmt.Errorf("failed to unmarshal schedule: %w", err)
		}
		if err := json.Unmarshal(payloadJSON, &job.Payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
		if err := json.Unmarshal(stateJSON, &job.State); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state: %w", err)
		}

		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

func (r *cronRepository) AddJob(ctx context.Context, job *repository.CronJob) error {
	scheduleJSON, err := json.Marshal(job.Schedule)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule: %w", err)
	}
	payloadJSON, err := json.Marshal(job.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	stateJSON, err := json.Marshal(job.State)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	query := `INSERT INTO cron_jobs (id, name, enabled, schedule, payload, state, created_at_ms, updated_at_ms, delete_after_run)
	          VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err = r.db.ExecContext(ctx, query,
		job.ID,
		job.Name,
		job.Enabled,
		scheduleJSON,
		payloadJSON,
		stateJSON,
		job.CreatedAtMS,
		job.UpdatedAtMS,
		job.DeleteAfterRun,
	)

	return err
}

func (r *cronRepository) UpdateJob(ctx context.Context, job *repository.CronJob) error {
	scheduleJSON, err := json.Marshal(job.Schedule)
	if err != nil {
		return fmt.Errorf("failed to marshal schedule: %w", err)
	}
	payloadJSON, err := json.Marshal(job.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	stateJSON, err := json.Marshal(job.State)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	query := `UPDATE cron_jobs
	          SET name = $2, enabled = $3, schedule = $4, payload = $5, state = $6, updated_at_ms = $7, delete_after_run = $8
	          WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query,
		job.ID,
		job.Name,
		job.Enabled,
		scheduleJSON,
		payloadJSON,
		stateJSON,
		job.UpdatedAtMS,
		job.DeleteAfterRun,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (r *cronRepository) DeleteJob(ctx context.Context, jobID string) error {
	query := `DELETE FROM cron_jobs WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, jobID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (r *cronRepository) GetDueJobs(ctx context.Context, now time.Time) ([]repository.CronJob, error) {
	nowMS := now.UnixMilli()

	query := `SELECT id, name, enabled, schedule, payload, state, created_at_ms, updated_at_ms, delete_after_run
	          FROM cron_jobs
	          WHERE enabled = true
	            AND (state->>'nextRunAtMs')::bigint <= $1
	          ORDER BY (state->>'nextRunAtMs')::bigint ASC`

	rows, err := r.db.QueryContext(ctx, query, nowMS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []repository.CronJob
	for rows.Next() {
		var job repository.CronJob
		var scheduleJSON, payloadJSON, stateJSON []byte

		if err := rows.Scan(
			&job.ID,
			&job.Name,
			&job.Enabled,
			&scheduleJSON,
			&payloadJSON,
			&stateJSON,
			&job.CreatedAtMS,
			&job.UpdatedAtMS,
			&job.DeleteAfterRun,
		); err != nil {
			return nil, err
		}

		// Unmarshal JSONB fields
		if err := json.Unmarshal(scheduleJSON, &job.Schedule); err != nil {
			return nil, fmt.Errorf("failed to unmarshal schedule: %w", err)
		}
		if err := json.Unmarshal(payloadJSON, &job.Payload); err != nil {
			return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
		}
		if err := json.Unmarshal(stateJSON, &job.State); err != nil {
			return nil, fmt.Errorf("failed to unmarshal state: %w", err)
		}

		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

func (r *cronRepository) UpdateJobState(ctx context.Context, jobID string, state repository.CronJobState) error {
	stateJSON, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	query := `UPDATE cron_jobs
	          SET state = $2, updated_at_ms = $3
	          WHERE id = $1`

	result, err := r.db.ExecContext(ctx, query, jobID, stateJSON, time.Now().UnixMilli())
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (r *cronRepository) GetNextWakeTime(ctx context.Context) (*time.Time, error) {
	query := `SELECT MIN((state->>'nextRunAtMs')::bigint)
	          FROM cron_jobs
	          WHERE enabled = true
	            AND state->>'nextRunAtMs' IS NOT NULL`

	var nextRunMS sql.NullInt64
	err := r.db.QueryRowContext(ctx, query).Scan(&nextRunMS)
	if err != nil {
		return nil, err
	}

	if !nextRunMS.Valid {
		return nil, nil // No jobs scheduled
	}

	t := time.UnixMilli(nextRunMS.Int64)
	return &t, nil
}
