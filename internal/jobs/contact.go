package jobs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/kom3da/pagewright/internal/db"
	"github.com/kom3da/pagewright/internal/db/dbgen"
	"github.com/kom3da/pagewright/internal/mail"
)

// ContactNotifyArgs tells a site that a contact form message arrived. The
// job holds only the submission's id: the message itself stays in
// form_submissions, so deleting it there leaves no copy in the job table.
type ContactNotifyArgs struct {
	TenantID     uuid.UUID `json:"tenant_id"`
	SubmissionID int64     `json:"submission_id"`
	To           string    `json:"to"`
	SiteName     string    `json:"site_name"`
}

// Kind implements river.JobArgs.
func (ContactNotifyArgs) Kind() string { return "contact_notify" }

// ContactNotifyWorker sends the notification with the submission as it is
// when the job runs; a submission deleted meanwhile is not sent.
type ContactNotifyWorker struct {
	river.WorkerDefaults[ContactNotifyArgs]
	db     *db.DB
	sender mail.Sender
}

// Timeout bounds one attempt.
func (w *ContactNotifyWorker) Timeout(*river.Job[ContactNotifyArgs]) time.Duration {
	return time.Minute
}

// Work implements river.Worker.
func (w *ContactNotifyWorker) Work(ctx context.Context, job *river.Job[ContactNotifyArgs]) error {
	var sub dbgen.FormSubmission
	err := w.db.InTenant(db.WithTenant(ctx, job.Args.TenantID), func(q *dbgen.Queries) error {
		var err error
		sub, err = q.GetFormSubmission(ctx, job.Args.SubmissionID)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // deleted before we got to it
	}
	if err != nil {
		return fmt.Errorf("contact_notify: load: %w", err)
	}
	return w.sender.Send(ctx, ContactNotification(job.Args.To, job.Args.SiteName, sub))
}

// ContactNotification is the mail telling a site about a submission.
func ContactNotification(to, siteName string, sub dbgen.FormSubmission) mail.Message {
	jst := time.FixedZone("JST", 9*60*60)
	return mail.Message{
		To:      to,
		Subject: "【" + siteName + "】お問い合わせがありました",
		Body: "サイトのお問い合わせフォームから、次の内容が届きました。\n" +
			"返信するときは、下のメールアドレスあてに送ってください。\n\n" +
			fmt.Sprintf("お名前：%s\nふりがな：%s\nメールアドレス：%s\n電話番号：%s\n\nお問い合わせ内容：\n%s\n",
				sub.Name, sub.Kana, sub.Email, sub.Phone, sub.Message) +
			"\n送信元のページ：" + sub.PagePath + "\n受付日時：" + sub.CreatedAt.In(jst).Format("2006年1月2日 15:04") + "\n",
	}
}
