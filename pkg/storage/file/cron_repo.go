package file

import (
	"context"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/cron"
	"github.com/sipeed/picoclaw/pkg/storage/repository"
)

type cronRepository struct {
	service *cron.CronService
}

// NewCronRepository creates a new file-based cron repository adapter.
// Note: This requires a CronService instance which is typically created at app startup.
func NewCronRepository(service *cron.CronService) repository.CronRepository {
	if service == nil {
		return &cronRepository{} // Return empty repo if service is nil
	}
	return &cronRepository{service: service}
}

func (r *cronRepository) GetJob(ctx context.Context, jobID string) (*repository.CronJob, error) {
	if r.service == nil {
		return nil, fmt.Errorf("cron service not initialized")
	}

	// CronService doesn't have GetJob method, so we list all and find by ID
	jobs := r.service.ListJobs(true)
	for _, job := range jobs {
		if job.ID == jobID {
			return convertToRepoCronJob(&job), nil
		}
	}

	return nil, fmt.Errorf("job not found: %s", jobID)
}

func (r *cronRepository) ListJobs(ctx context.Context, includeDisabled bool) ([]repository.CronJob, error) {
	if r.service == nil {
		return []repository.CronJob{}, nil
	}

	fileJobs := r.service.ListJobs(includeDisabled)

	repoJobs := make([]repository.CronJob, len(fileJobs))
	for i, fj := range fileJobs {
		repoJobs[i] = *convertToRepoCronJob(&fj)
	}

	return repoJobs, nil
}

func (r *cronRepository) AddJob(ctx context.Context, job *repository.CronJob) error {
	if r.service == nil {
		return fmt.Errorf("cron service not initialized")
	}

	// CronService.AddJob signature: (name, schedule, message, deliver, channel, to) - we need to adapt
	_, err := r.service.AddJob(
		job.Name,
		cron.CronSchedule{
			Kind:    job.Schedule.Kind,
			AtMS:    job.Schedule.AtMS,
			EveryMS: job.Schedule.EveryMS,
			Expr:    job.Schedule.Expr,
			TZ:      job.Schedule.TZ,
		},
		job.Payload.Message,
		job.Payload.Deliver,
		job.Payload.Channel,
		job.Payload.To,
	)
	return err
}

func (r *cronRepository) UpdateJob(ctx context.Context, job *repository.CronJob) error {
	if r.service == nil {
		return fmt.Errorf("cron service not initialized")
	}

	// CronService doesn't have UpdateJob - need to remove and re-add
	r.service.RemoveJob(job.ID)
	return r.AddJob(ctx, job)
}

func (r *cronRepository) DeleteJob(ctx context.Context, jobID string) error {
	if r.service == nil {
		return fmt.Errorf("cron service not initialized")
	}

	if !r.service.RemoveJob(jobID) {
		return fmt.Errorf("job not found: %s", jobID)
	}
	return nil
}

func (r *cronRepository) GetDueJobs(ctx context.Context, now time.Time) ([]repository.CronJob, error) {
	if r.service == nil {
		return []repository.CronJob{}, nil
	}

	// CronService doesn't expose GetDueJobs publicly, but it handles scheduling internally
	// For file-based storage, we return empty slice as cron service manages this internally
	return []repository.CronJob{}, nil
}

func (r *cronRepository) UpdateJobState(ctx context.Context, jobID string, state repository.CronJobState) error {
	if r.service == nil {
		return fmt.Errorf("cron service not initialized")
	}

	// CronService manages state internally during execution
	// For file-based storage, this is a no-op as state is updated automatically
	return nil
}

func (r *cronRepository) GetNextWakeTime(ctx context.Context) (*time.Time, error) {
	if r.service == nil {
		return nil, nil
	}

	// CronService manages wake times internally
	// For file-based storage, we return nil as service handles this
	return nil, nil
}

// Helper function to convert from cron.CronJob to repository.CronJob
func convertToRepoCronJob(fileJob *cron.CronJob) *repository.CronJob {
	if fileJob == nil {
		return nil
	}

	return &repository.CronJob{
		ID:      fileJob.ID,
		Name:    fileJob.Name,
		Enabled: fileJob.Enabled,
		Schedule: repository.CronSchedule{
			Kind:    fileJob.Schedule.Kind,
			AtMS:    fileJob.Schedule.AtMS,
			EveryMS: fileJob.Schedule.EveryMS,
			Expr:    fileJob.Schedule.Expr,
			TZ:      fileJob.Schedule.TZ,
		},
		Payload: repository.CronPayload{
			Kind:    fileJob.Payload.Kind,
			Message: fileJob.Payload.Message,
			Deliver: fileJob.Payload.Deliver,
			Channel: fileJob.Payload.Channel,
			To:      fileJob.Payload.To,
		},
		State: repository.CronJobState{
			NextRunAtMS: fileJob.State.NextRunAtMS,
			LastRunAtMS: fileJob.State.LastRunAtMS,
			LastStatus:  fileJob.State.LastStatus,
			LastError:   fileJob.State.LastError,
		},
		CreatedAtMS:    fileJob.CreatedAtMS,
		UpdatedAtMS:    fileJob.UpdatedAtMS,
		DeleteAfterRun: fileJob.DeleteAfterRun,
	}
}
