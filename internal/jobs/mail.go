package jobs

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/kom3da/pagewright/internal/mail"
)

// SendMailArgs sends one mail. Sending from a job keeps SMTP out of
// requests and retries when the server is down. The job row holds the mail
// until River deletes finished jobs (a day by default).
type SendMailArgs struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

// Kind implements river.JobArgs.
func (SendMailArgs) Kind() string { return "send_mail" }

// SendMailWorker sends mail with a mail.Sender.
type SendMailWorker struct {
	river.WorkerDefaults[SendMailArgs]
	sender mail.Sender
}

// Timeout bounds one attempt; the sender also has its own limit.
func (w *SendMailWorker) Timeout(*river.Job[SendMailArgs]) time.Duration { return time.Minute }

// Work implements river.Worker.
func (w *SendMailWorker) Work(ctx context.Context, job *river.Job[SendMailArgs]) error {
	return w.sender.Send(ctx, mail.Message{To: job.Args.To, Subject: job.Args.Subject, Body: job.Args.Body})
}
