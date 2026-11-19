package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nico9julio/PGuardian/internal/database"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func setupTestDispatcher(t *testing.T, tgRule database.NotificationRule) (*Dispatcher, *database.Store, func() []string) {
	t.Helper()
	store, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.SetEncryptedSetting(database.KeyTelegramBotTokenEnc, "test-token"); err != nil {
		t.Fatalf("set token: %v", err)
	}
	if err := store.SaveTelegramRecipient(database.TelegramRecipient{
		ChatID:  "12345",
		Label:   "Test",
		Enabled: true,
	}); err != nil {
		t.Fatalf("save recipient: %v", err)
	}
	if err := store.SaveNotificationRule(tgRule); err != nil {
		t.Fatalf("save rule: %v", err)
	}

	b := NewBroadcaster()
	d := NewDispatcher(store, b)

	var mu sync.Mutex
	var sent []string
	d.telegram.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		var payload struct {
			Text string `json:"text"`
		}
		_ = json.Unmarshal(body, &payload)
		mu.Lock()
		sent = append(sent, payload.Text)
		mu.Unlock()
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			Header:     make(http.Header),
		}, nil
	})

	getSent := func() []string {
		deadline := time.Now().Add(500 * time.Millisecond)
		for time.Now().Before(deadline) {
			mu.Lock()
			n := len(sent)
			mu.Unlock()
			if n > 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		time.Sleep(25 * time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		out := make([]string, len(sent))
		copy(out, sent)
		return out
	}

	return d, store, getSent
}

func TestSendDailySummary_NoRedCrossWhenZeroFailures(t *testing.T) {
	d, _, getSent := setupTestDispatcher(t, database.NotificationRule{
		Channel:      "telegram",
		DailySummary: true,
	})

	d.SendDailySummary(database.DashboardMetrics{
		TotalExecutions24h: 4,
		Success24h:         4,
		Failed24h:          0,
		PendingCloudCount:  0,
		TotalLocalBytes:    1024 * 1024 * 512,
	})

	msgs := getSent()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 telegram message, got %d", len(msgs))
	}
	if strings.Contains(msgs[0], "❌") {
		t.Fatalf("expected no ❌ in daily summary when Failed24h == 0, got: %s", msgs[0])
	}
	if !strings.Contains(msgs[0], "✅ 4") {
		t.Fatalf("expected success count in daily summary, got: %s", msgs[0])
	}
}

func TestSendDailySummary_ShowsRedCrossWhenFailuresExist(t *testing.T) {
	d, _, getSent := setupTestDispatcher(t, database.NotificationRule{
		Channel:      "telegram",
		DailySummary: true,
	})

	d.SendDailySummary(database.DashboardMetrics{
		TotalExecutions24h: 5,
		Success24h:         3,
		Failed24h:          2,
	})

	msgs := getSent()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 telegram message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0], "❌ 2") {
		t.Fatalf("expected ❌ 2 in daily summary when Failed24h == 2, got: %s", msgs[0])
	}
}

func TestNotifySuccessMode_PerBackupVsTaskSummary(t *testing.T) {
	d, _, getSent := setupTestDispatcher(t, database.NotificationRule{
		Channel:     "telegram",
		OnSuccess:   true,
		SuccessMode: database.SuccessModeTaskSummary,
	})

	subID, ch := d.Broadcaster().Subscribe()
	defer d.Broadcaster().Unsubscribe(subID)

	// In batch task mode with task_summary, per-backup Telegram should be suppressed,
	// while live broadcaster event still fires.
	batchCtx := WithBatchTask(t.Context())
	d.NotifyBackupSuccessCtx(batchCtx, "db1", "Nightly", 10*1024*1024, true, 1)

	select {
	case ev := <-ch:
		if ev.Type != EventBackupCompleted {
			t.Fatalf("expected EventBackupCompleted, got %s", ev.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("expected live event on broadcaster")
	}

	time.Sleep(60 * time.Millisecond)
	if msgs := getSent(); len(msgs) != 0 {
		t.Fatalf("expected 0 per-backup telegram messages in task_summary mode, got %d: %v", len(msgs), msgs)
	}

	// Task summary without errors: sends 1 message without ❌.
	d.NotifyTaskSummary(TaskSummary{
		ProfileName:  "Nightly",
		TriggerType:  "scheduled",
		TargetType:   "database",
		DriveEnabled: true,
		Duration:     12 * time.Second,
		Items: []TaskBackupItem{
			{Name: "db1", SizeBytes: 10 * 1024 * 1024, Success: true, CloudUploaded: true},
			{Name: "db2", SizeBytes: 20 * 1024 * 1024, Success: true, CloudUploaded: true},
		},
	})

	msgs := getSent()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 task summary message, got %d", len(msgs))
	}
	if strings.Contains(msgs[0], "❌") {
		t.Fatalf("expected no ❌ in clean task summary, got: %s", msgs[0])
	}
	if !strings.Contains(msgs[0], "2/2") || !strings.Contains(msgs[0], "db1") || !strings.Contains(msgs[0], "db2") {
		t.Fatalf("expected summary details in message, got: %s", msgs[0])
	}
}

func TestNotificationRuleSuccessModePersistence(t *testing.T) {
	store, err := database.Open(filepath.Join(t.TempDir(), "persist.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer func() { _ = store.Close() }()

	if err := store.SaveNotificationRule(database.NotificationRule{
		Channel:     "telegram",
		OnSuccess:   true,
		SuccessMode: database.SuccessModeTaskSummary,
	}); err != nil {
		t.Fatalf("save rule: %v", err)
	}

	rules, err := store.GetNotificationRules()
	if err != nil {
		t.Fatalf("get rules: %v", err)
	}
	var found bool
	for _, r := range rules {
		if r.Channel == "telegram" {
			found = true
			if r.SuccessMode != database.SuccessModeTaskSummary {
				t.Fatalf("expected %s, got %s", database.SuccessModeTaskSummary, r.SuccessMode)
			}
		}
	}
	if !found {
		t.Fatal("telegram rule not found")
	}
}
