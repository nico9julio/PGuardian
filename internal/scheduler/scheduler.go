package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/nico9julio/PGuardian/internal/backup"
	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/database"
	"github.com/nico9julio/PGuardian/internal/notify"
	"github.com/robfig/cron/v3"
)

type Coordinator struct {
	mu         sync.Mutex
	store      *database.Store
	engine     *backup.Engine
	dispatcher *notify.Dispatcher
	cronRunner *cron.Cron
	cancel     context.CancelFunc
	paused     bool
}

func NewCoordinator(store *database.Store, engine *backup.Engine, dispatcher *notify.Dispatcher) *Coordinator {
	return &Coordinator{
		store:      store,
		engine:     engine,
		dispatcher: dispatcher,
	}
}

func (c *Coordinator) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.cancel = cancel
	c.mu.Unlock()

	if err := c.Reload(runCtx); err != nil {
		slog.Warn("initial cron reload warning", "error", err)
	}

	config.SafeGo("cloud-retry-and-daily-summary", func() {
		ticker := time.NewTicker(45 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				c.tickMaintenance(runCtx)
			}
		}
	})
	return nil
}

func (c *Coordinator) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if c.cronRunner != nil {
		ctx := c.cronRunner.Stop()
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
		c.cronRunner = nil
	}
}

func (c *Coordinator) SetPaused(paused bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.paused = paused
}

func (c *Coordinator) IsPaused() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.paused
}

// Reload rebuilds all cron schedules from SQLite backup_profiles.
func (c *Coordinator) Reload(ctx context.Context) error {
	profiles, err := c.store.ListProfiles()
	if err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cronRunner != nil {
		c.cronRunner.Stop()
	}

	cr := cron.New()
	for _, p := range profiles {
		if !p.Enabled || p.ScheduleCron == "" {
			continue
		}
		profID := p.ID
		profName := p.Name
		_, addErr := cr.AddFunc(p.ScheduleCron, func() {
			if c.IsPaused() {
				return
			}
			config.SafeGo("scheduled-profile", func() {
				slog.Info("executing scheduled backup profile", "profileId", profID, "name", profName)
				if err := c.engine.RunProfile(ctx, profID, "scheduled"); err != nil {
					slog.Error("scheduled profile failed", "profileId", profID, "error", err)
				}
			})
		})
		if addErr != nil {
			slog.Error("invalid cron expression in profile", "profileId", profID, "cron", p.ScheduleCron, "error", addErr)
		}
	}
	cr.Start()
	c.cronRunner = cr
	return nil
}

// ValidateCron checks whether a 5-field cron expression is valid.
func ValidateCron(expr string) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	_, err := parser.Parse(expr)
	return err
}

func (c *Coordinator) tickMaintenance(ctx context.Context) {
	if c.IsPaused() {
		return
	}

	// 1. Drain due cloud uploads from SQLite persistent queue
	due, err := c.store.ListDueCloudUploads(time.Now())
	if err == nil {
		for _, ex := range due {
			if c.IsPaused() {
				return
			}
			_ = c.engine.UploadExecutionToCloud(ctx, ex.ID)
		}
	}

	// 2. Check Daily Summary schedule
	c.checkDailySummary()
}

func (c *Coordinator) checkDailySummary() {
	rules, err := c.store.GetNotificationRules()
	if err != nil {
		return
	}
	var targetTime string
	anyEnabled := false
	for _, r := range rules {
		if r.DailySummary {
			anyEnabled = true
			if r.DailySummaryTime != "" {
				targetTime = r.DailySummaryTime
			}
		}
	}
	if !anyEnabled || targetTime == "" {
		return
	}

	nowLocal := time.Now()
	todayKey := nowLocal.Format("2006-01-02")
	if c.store.GetSetting(database.KeyLastDailySummaryDate, "") == todayKey {
		return
	}

	currentHHMM := nowLocal.Format("15:04")
	if currentHHMM >= targetTime {
		_ = c.store.SetSetting(database.KeyLastDailySummaryDate, todayKey)
		if metrics, mErr := c.store.GetDashboardMetrics(); mErr == nil {
			c.dispatcher.SendDailySummary(metrics)
		}
	}
}
