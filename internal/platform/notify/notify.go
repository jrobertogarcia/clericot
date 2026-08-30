package notify

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"github.com/wneessen/go-mail"
)

const (
	TypeEmailNotification = "notify:email"
)

type EmailMessage struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	IsHTML  bool   `json:"is_html"`
}

type NotificationDispatcher struct {
	asynqClient *asynq.Client
	mailClient  *mail.Client
	fromAddress string
}

func NewNotificationDispatcher(asynqClient *asynq.Client, fromAddress string) *NotificationDispatcher {
	return &NotificationDispatcher{
		asynqClient: asynqClient,
		fromAddress: fromAddress,
	}
}

// SetMailClient configures the underlying go-mail client.
func (d *NotificationDispatcher) SetMailClient(client *mail.Client) {
	d.mailClient = client
}

// EnqueueEmail enqueues an email sending task into Asynq.
func (d *NotificationDispatcher) EnqueueEmail(ctx context.Context, msg EmailMessage) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to serialize email payload: %w", err)
	}

	task := asynq.NewTask(TypeEmailNotification, payload)
	if d.asynqClient != nil {
		_, err = d.asynqClient.EnqueueContext(ctx, task)
		if err != nil {
			return fmt.Errorf("failed to enqueue email task: %w", err)
		}
	}

	return nil
}

// SendDirect sends an email immediately without queuing (used inside workers).
func (d *NotificationDispatcher) SendDirect(ctx context.Context, msg EmailMessage) error {
	if d.mailClient == nil {
		return nil // Dev/test mock
	}

	m := mail.NewMsg()
	if err := m.From(d.fromAddress); err != nil {
		return fmt.Errorf("failed to set from address: %w", err)
	}
	if err := m.To(msg.To); err != nil {
		return fmt.Errorf("failed to set to address: %w", err)
	}
	m.Subject(msg.Subject)
	if msg.IsHTML {
		m.SetBodyString(mail.TypeTextHTML, msg.Body)
	} else {
		m.SetBodyString(mail.TypeTextPlain, msg.Body)
	}

	return d.mailClient.DialAndSendWithContext(ctx, m)
}
