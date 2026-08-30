package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
	"github.com/riverqueue/river"
)

const (
	TypeUserWelcomeEmail = "auth:user_welcome_email"
)

// UserRegisteredJobArgs defines the River job arguments for when a user registers.
type UserRegisteredJobArgs struct {
	UserID   string `json:"user_id"`
	TenantID string `json:"tenant_id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
}

// Kind implements river.JobArgs.
func (UserRegisteredJobArgs) Kind() string {
	return "auth.user_registered.v1"
}

// UserRegisteredWorker processes user registered River events.
type UserRegisteredWorker struct {
	river.WorkerDefaults[UserRegisteredJobArgs]
	logger *slog.Logger
}

// NewUserRegisteredWorker creates a new UserRegisteredWorker.
func NewUserRegisteredWorker(logger *slog.Logger) *UserRegisteredWorker {
	if logger == nil {
		logger = slog.Default()
	}
	return &UserRegisteredWorker{logger: logger}
}

// Work handles the execution of UserRegisteredJobArgs.
func (w *UserRegisteredWorker) Work(ctx context.Context, job *river.Job[UserRegisteredJobArgs]) error {
	w.logger.Info("processing user registered background event",
		"user_id", job.Args.UserID,
		"tenant_id", job.Args.TenantID,
		"email", job.Args.Email,
	)
	return nil
}

// ProcessWelcomeEmailTask handles Asynq welcome email task processing.
func ProcessWelcomeEmailTask(ctx context.Context, t *asynq.Task) error {
	var p struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.Unmarshal(t.Payload(), &p); err != nil {
		return fmt.Errorf("failed to parse welcome email task payload: %w", err)
	}

	slog.Info("sending welcome notification to new user", "email", p.Email, "name", p.Name)
	return nil
}
