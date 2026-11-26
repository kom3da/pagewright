package jobs

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/kom3da/pagewright/internal/db/dbtest"
	"github.com/kom3da/pagewright/internal/mail"
)

type recordSender struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (s *recordSender) Send(_ context.Context, m mail.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sent = append(s.sent, m)
	return nil
}

// TestContactNotify sends the submission as stored when the job runs, and
// nothing once it has been deleted.
func TestContactNotify(t *testing.T) {
	env := dbtest.New(t)
	tenant := env.CreateTenant(t, "通知株式会社")
	other := env.CreateTenant(t, "別の会社")
	ctx := context.Background()
	var id int64
	if err := env.Admin.QueryRow(ctx, `INSERT INTO form_submissions (tenant_id, name, kana, email, phone, message, page_path)
		VALUES ($1, '山田', 'やまだ', 'y@example.com', '', 'お湯が出ません', '/contact/') RETURNING id`, tenant).Scan(&id); err != nil {
		t.Fatal(err)
	}
	sender := &recordSender{}
	w := &ContactNotifyWorker{db: env.DB, sender: sender}
	run := func(tenantID [16]byte) error {
		args := ContactNotifyArgs{TenantID: tenantID, SubmissionID: id, To: "site@example.com", SiteName: "通知株式会社"}
		return w.Work(ctx, &river.Job[ContactNotifyArgs]{JobRow: &rivertype.JobRow{}, Args: args})
	}

	if err := run(tenant); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 || !strings.Contains(sender.sent[0].Body, "お湯が出ません") {
		t.Fatalf("sent = %+v, want one notification with the message", sender.sent)
	}
	// Another tenant's job cannot read it.
	if err := run(other); err != nil || len(sender.sent) != 1 {
		t.Errorf("other tenant: err = %v, sent %d, want nothing sent", err, len(sender.sent))
	}
	if _, err := env.Admin.Exec(ctx, "DELETE FROM form_submissions WHERE id = $1", id); err != nil {
		t.Fatal(err)
	}
	if err := run(tenant); err != nil || len(sender.sent) != 1 {
		t.Errorf("after delete: err = %v, sent %d, want nothing more sent", err, len(sender.sent))
	}
}
