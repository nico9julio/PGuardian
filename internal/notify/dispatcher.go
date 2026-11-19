package notify

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/database"
)

type batchTaskCtxKey struct{}

// WithBatchTask marks a context as part of a multi-target profile execution so
// per-backup success notifications can be deferred to the final task summary.
func WithBatchTask(ctx context.Context) context.Context {
	return context.WithValue(ctx, batchTaskCtxKey{}, true)
}

func isBatchTask(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(batchTaskCtxKey{}).(bool)
	return v
}

type TaskBackupItem struct {
	Name          string
	SizeBytes     int64
	Success       bool
	CloudUploaded bool
	CloudPending  bool
	Warning       string
	Error         string
}

type TaskSummary struct {
	ProfileName  string
	TriggerType  string
	TargetType   string
	DriveEnabled bool
	Duration     time.Duration
	Items        []TaskBackupItem
}

type Dispatcher struct {
	store       *database.Store
	broadcaster *Broadcaster
	telegram    *TelegramClient
	hostname    string
}

func NewDispatcher(store *database.Store, broadcaster *Broadcaster) *Dispatcher {
	h, _ := os.Hostname()
	if h == "" {
		h = "WindowsServer"
	}
	return &Dispatcher{
		store:       store,
		broadcaster: broadcaster,
		telegram:    NewTelegramClient(),
		hostname:    h,
	}
}

func (d *Dispatcher) Broadcaster() *Broadcaster {
	return d.broadcaster
}

func (d *Dispatcher) Telegram() *TelegramClient {
	return d.telegram
}

func (d *Dispatcher) isSpanish() bool {
	return d.store.GetSetting(database.KeyLanguage, "es") != "en"
}

func (d *Dispatcher) ruleMap() map[string]database.NotificationRule {
	rules, _ := d.store.GetNotificationRules()
	m := make(map[string]database.NotificationRule, 2)
	for _, r := range rules {
		m[r.Channel] = r
	}
	return m
}

func (d *Dispatcher) EmitLive(ev Event) {
	d.broadcaster.Publish(ev)
}

func (d *Dispatcher) NotifyBackupSuccess(dbName, profileName string, sizeBytes int64, cloudUploaded bool, execID int64) {
	d.NotifyBackupSuccessWithWarningCtx(context.Background(), dbName, profileName, sizeBytes, cloudUploaded, "", execID)
}

func (d *Dispatcher) NotifyBackupSuccessCtx(ctx context.Context, dbName, profileName string, sizeBytes int64, cloudUploaded bool, execID int64) {
	d.NotifyBackupSuccessWithWarningCtx(ctx, dbName, profileName, sizeBytes, cloudUploaded, "", execID)
}

func (d *Dispatcher) NotifyBackupSuccessWithWarning(targetName, profileName string, sizeBytes int64, cloudUploaded bool, warningMsg string, execID int64) {
	d.NotifyBackupSuccessWithWarningCtx(context.Background(), targetName, profileName, sizeBytes, cloudUploaded, warningMsg, execID)
}

func (d *Dispatcher) NotifyBackupSuccessWithWarningCtx(ctx context.Context, targetName, profileName string, sizeBytes int64, cloudUploaded bool, warningMsg string, execID int64) {
	rules := d.ruleMap()
	winRule := rules["windows"]
	tgRule := rules["telegram"]

	sizeText := formatBytesHuman(sizeBytes)
	severity := "success"
	if warningMsg != "" {
		severity = "warning"
	}

	var title, msg, tgMsg string
	if d.isSpanish() {
		title = "Copia completada: " + targetName
		if cloudUploaded {
			msg = fmt.Sprintf("Perfil '%s' completado (%s) en Local y Google Drive.", profileName, sizeText)
		} else {
			msg = fmt.Sprintf("Perfil '%s' completado localmente (%s).", profileName, sizeText)
		}
		if warningMsg != "" {
			msg += " Aviso: " + warningMsg
		}
		header := "✅ <b>Backup completado [%s]</b>"
		if warningMsg != "" {
			header = "⚠️ <b>Backup completado con aviso [%s]</b>"
		}
		tgMsg = fmt.Sprintf(header+"\n<b>Origen:</b> <code>%s</code>\n<b>Perfil:</b> %s\n<b>Tamaño:</b> %s\n<b>Destino:</b> %s",
			html.EscapeString(d.hostname), html.EscapeString(targetName), html.EscapeString(profileName), sizeText,
			map[bool]string{true: "Local + Google Drive", false: "Local"}[cloudUploaded])
		if warningMsg != "" {
			tgMsg += fmt.Sprintf("\n⚠️ <b>Aviso:</b> <code>%s</code>", html.EscapeString(warningMsg))
		}
	} else {
		title = "Backup completed: " + targetName
		if cloudUploaded {
			msg = fmt.Sprintf("Profile '%s' completed (%s) on Local & Google Drive.", profileName, sizeText)
		} else {
			msg = fmt.Sprintf("Profile '%s' completed locally (%s).", profileName, sizeText)
		}
		if warningMsg != "" {
			msg += " Warning: " + warningMsg
		}
		header := "✅ <b>Backup completed [%s]</b>"
		if warningMsg != "" {
			header = "⚠️ <b>Backup completed with warning [%s]</b>"
		}
		tgMsg = fmt.Sprintf(header+"\n<b>Target:</b> <code>%s</code>\n<b>Profile:</b> %s\n<b>Size:</b> %s\n<b>Destination:</b> %s",
			html.EscapeString(d.hostname), html.EscapeString(targetName), html.EscapeString(profileName), sizeText,
			map[bool]string{true: "Local + Google Drive", false: "Local"}[cloudUploaded])
		if warningMsg != "" {
			tgMsg += fmt.Sprintf("\n⚠️ <b>Warning:</b> <code>%s</code>", html.EscapeString(warningMsg))
		}
	}

	batch := isBatchTask(ctx)
	showWinToast := winRule.OnSuccess && !(batch && winRule.SuccessMode == database.SuccessModeTaskSummary)

	d.broadcaster.Publish(Event{
		Type:        EventBackupCompleted,
		Title:       title,
		Message:     msg,
		Severity:    severity,
		ShowToast:   showWinToast,
		ExecutionID: execID,
		DBName:      targetName,
	})

	if tgRule.OnSuccess && !(batch && tgRule.SuccessMode == database.SuccessModeTaskSummary) {
		d.sendTelegramAsync(tgMsg)
	}
}

func (d *Dispatcher) NotifyBackupError(dbName, profileName, errDetail string, execID int64) {
	rules := d.ruleMap()
	winRule := rules["windows"]
	tgRule := rules["telegram"]

	var title, msg, tgMsg string
	if d.isSpanish() {
		title = "Error de Backup: " + dbName
		msg = fmt.Sprintf("Fallo en '%s': %s", profileName, errDetail)
		tgMsg = fmt.Sprintf("🚨 <b>PGuardian Error [%s]</b>\n<b>Base:</b> <code>%s</code>\n<b>Perfil:</b> %s\n<b>Detalle:</b> <code>%s</code>",
			html.EscapeString(d.hostname), html.EscapeString(dbName), html.EscapeString(profileName), html.EscapeString(errDetail))
	} else {
		title = "Backup Failed: " + dbName
		msg = fmt.Sprintf("Failed in '%s': %s", profileName, errDetail)
		tgMsg = fmt.Sprintf("🚨 <b>PGuardian Error [%s]</b>\n<b>Database:</b> <code>%s</code>\n<b>Profile:</b> %s\n<b>Detail:</b> <code>%s</code>",
			html.EscapeString(d.hostname), html.EscapeString(dbName), html.EscapeString(profileName), html.EscapeString(errDetail))
	}

	d.broadcaster.Publish(Event{
		Type:        EventBackupFailed,
		Title:       title,
		Message:     msg,
		Severity:    "error",
		ShowToast:   winRule.OnError,
		ExecutionID: execID,
		DBName:      dbName,
	})

	if tgRule.OnError {
		d.sendTelegramAsync(tgMsg)
	}
}

func (d *Dispatcher) NotifyCloudRetry(dbName string, retryNum int, nextRetry string, errDetail string, execID int64) {
	rules := d.ruleMap()
	winRule := rules["windows"]
	tgRule := rules["telegram"]

	var title, msg, tgMsg string
	if d.isSpanish() {
		title = "Subida a Drive en cola: " + dbName
		msg = fmt.Sprintf("Copia local segura. Reintento #%d programado (%s).", retryNum, nextRetry)
		tgMsg = fmt.Sprintf("⚠️ <b>PGuardian Reintento Nube [%s]</b>\n<b>Base:</b> <code>%s</code>\nCopia local guardada. Subida a Google Drive en reintento #%d (%s).\n<code>%s</code>",
			html.EscapeString(d.hostname), html.EscapeString(dbName), retryNum, html.EscapeString(nextRetry), html.EscapeString(errDetail))
	} else {
		title = "Drive upload queued: " + dbName
		msg = fmt.Sprintf("Local copy safe. Retry #%d scheduled (%s).", retryNum, nextRetry)
		tgMsg = fmt.Sprintf("⚠️ <b>PGuardian Cloud Retry [%s]</b>\n<b>Database:</b> <code>%s</code>\nLocal copy safe. Google Drive upload retry #%d (%s).\n<code>%s</code>",
			html.EscapeString(d.hostname), html.EscapeString(dbName), retryNum, html.EscapeString(nextRetry), html.EscapeString(errDetail))
	}

	d.broadcaster.Publish(Event{
		Type:        EventCloudRetrying,
		Title:       title,
		Message:     msg,
		Severity:    "warning",
		ShowToast:   winRule.OnRetry,
		ExecutionID: execID,
		DBName:      dbName,
	})

	if tgRule.OnRetry && retryNum == 1 {
		d.sendTelegramAsync(tgMsg)
	}
}

func (d *Dispatcher) NotifyLowDisk(path string, freeGB, thresholdGB float64) {
	rules := d.ruleMap()
	winRule := rules["windows"]
	tgRule := rules["telegram"]

	var title, msg, tgMsg string
	if d.isSpanish() {
		title = "Alerta de Espacio en Disco"
		msg = fmt.Sprintf("Quedan %.2f GB libres en %s (umbral: %.0f GB).", freeGB, path, thresholdGB)
		tgMsg = fmt.Sprintf("💾 <b>PGuardian Alerta Disco [%s]</b>\nEspacio libre bajo en <code>%s</code>: <b>%.2f GB</b> (mínimo configurado: %.0f GB).",
			html.EscapeString(d.hostname), html.EscapeString(path), freeGB, thresholdGB)
	} else {
		title = "Low Disk Space Warning"
		msg = fmt.Sprintf("Only %.2f GB free on %s (threshold: %.0f GB).", freeGB, path, thresholdGB)
		tgMsg = fmt.Sprintf("💾 <b>PGuardian Low Disk [%s]</b>\nLow free space on <code>%s</code>: <b>%.2f GB</b> (threshold: %.0f GB).",
			html.EscapeString(d.hostname), html.EscapeString(path), freeGB, thresholdGB)
	}

	d.broadcaster.Publish(Event{
		Type:      EventLowDiskSpace,
		Title:     title,
		Message:   msg,
		Severity:  "warning",
		ShowToast: winRule.OnLowDisk,
	})

	if tgRule.OnLowDisk {
		d.sendTelegramAsync(tgMsg)
	}
}

func (d *Dispatcher) SendDailySummary(metrics database.DashboardMetrics) {
	rules := d.ruleMap()
	winRule := rules["windows"]
	tgRule := rules["telegram"]

	totalGB := float64(metrics.TotalLocalBytes) / (1024 * 1024 * 1024)
	var title, msg, tgMsg string
	if d.isSpanish() {
		title = "Resumen Diario PGuardian"
		if metrics.Failed24h > 0 {
			msg = fmt.Sprintf("24h: %d exitosas, %d fallidas, %d pendientes nube.", metrics.Success24h, metrics.Failed24h, metrics.PendingCloudCount)
		} else {
			msg = fmt.Sprintf("24h: %d exitosas, %d pendientes nube.", metrics.Success24h, metrics.PendingCloudCount)
		}
		statusLine := fmt.Sprintf("✅ %d exitosas", metrics.Success24h)
		if metrics.Failed24h > 0 {
			statusLine += fmt.Sprintf(" | ❌ %d con error", metrics.Failed24h)
		}
		tgMsg = fmt.Sprintf("📊 <b>Resumen Diario PGuardian [%s]</b>\n• Copias 24h: <b>%d</b> (%s)\n• Cola Google Drive: <b>%d</b>\n• Espacio local usado: <b>%.2f GB</b>",
			html.EscapeString(d.hostname), metrics.TotalExecutions24h, statusLine, metrics.PendingCloudCount, totalGB)
	} else {
		title = "PGuardian Daily Summary"
		if metrics.Failed24h > 0 {
			msg = fmt.Sprintf("24h: %d succeeded, %d failed, %d cloud pending.", metrics.Success24h, metrics.Failed24h, metrics.PendingCloudCount)
		} else {
			msg = fmt.Sprintf("24h: %d succeeded, %d cloud pending.", metrics.Success24h, metrics.PendingCloudCount)
		}
		statusLine := fmt.Sprintf("✅ %d succeeded", metrics.Success24h)
		if metrics.Failed24h > 0 {
			statusLine += fmt.Sprintf(" | ❌ %d failed", metrics.Failed24h)
		}
		tgMsg = fmt.Sprintf("📊 <b>PGuardian Daily Summary [%s]</b>\n• Backups 24h: <b>%d</b> (%s)\n• Drive Queue: <b>%d</b>\n• Local storage used: <b>%.2f GB</b>",
			html.EscapeString(d.hostname), metrics.TotalExecutions24h, statusLine, metrics.PendingCloudCount, totalGB)
	}

	if winRule.DailySummary {
		d.broadcaster.Publish(Event{
			Type:      EventTestNotification,
			Title:     title,
			Message:   msg,
			Severity:  "info",
			ShowToast: true,
		})
	}
	if tgRule.DailySummary {
		d.sendTelegramAsync(tgMsg)
	}
}

func (d *Dispatcher) NotifyTaskSummary(summary TaskSummary) {
	if len(summary.Items) == 0 {
		return
	}
	rules := d.ruleMap()
	winRule := rules["windows"]
	tgRule := rules["telegram"]

	winWants := winRule.OnSuccess && winRule.SuccessMode == database.SuccessModeTaskSummary
	tgWants := tgRule.OnSuccess && tgRule.SuccessMode == database.SuccessModeTaskSummary
	if !winWants && !tgWants {
		return
	}

	var successCount, failedCount, warningCount, cloudUploadedCount, cloudPendingCount int
	var totalBytes int64
	for _, item := range summary.Items {
		if item.Success {
			successCount++
			totalBytes += item.SizeBytes
			if item.CloudUploaded {
				cloudUploadedCount++
			}
			if item.CloudPending {
				cloudPendingCount++
			}
			if item.Warning != "" {
				warningCount++
			}
		} else {
			failedCount++
		}
	}

	totalCount := len(summary.Items)
	sizeText := formatBytesHuman(totalBytes)
	durText := formatDurationShort(summary.Duration)
	es := d.isSpanish()

	triggerLabel := summary.TriggerType
	if es {
		if summary.TriggerType == "scheduled" {
			triggerLabel = "Programado"
		} else if summary.TriggerType == "manual" {
			triggerLabel = "Manual"
		}
	} else {
		if summary.TriggerType == "scheduled" {
			triggerLabel = "Scheduled"
		} else if summary.TriggerType == "manual" {
			triggerLabel = "Manual"
		}
	}

	var destLabel string
	if !summary.DriveEnabled {
		destLabel = "Local"
	} else if cloudPendingCount > 0 && cloudUploadedCount == 0 {
		if es {
			destLabel = fmt.Sprintf("Local (%d en cola Google Drive)", cloudPendingCount)
		} else {
			destLabel = fmt.Sprintf("Local (%d queued for Google Drive)", cloudPendingCount)
		}
	} else if cloudPendingCount > 0 {
		if es {
			destLabel = fmt.Sprintf("Local + Google Drive (%d en cola)", cloudPendingCount)
		} else {
			destLabel = fmt.Sprintf("Local + Google Drive (%d queued)", cloudPendingCount)
		}
	} else {
		destLabel = "Local + Google Drive"
	}

	severity := "success"
	if failedCount > 0 {
		severity = "error"
	} else if warningCount > 0 || cloudPendingCount > 0 {
		severity = "warning"
	}

	var title, msg, tgHeader string
	if es {
		switch {
		case failedCount > 0:
			title = fmt.Sprintf("Tarea finalizada con errores: %s", summary.ProfileName)
			msg = fmt.Sprintf("%d/%d exitosos, %d con error (%s en %s).", successCount, totalCount, failedCount, sizeText, durText)
			tgHeader = "❌ <b>Tarea de Backup Finalizada con Errores [%s]</b>"
		case warningCount > 0 || cloudPendingCount > 0:
			title = fmt.Sprintf("Tarea completada con avisos: %s", summary.ProfileName)
			msg = fmt.Sprintf("%d/%d exitosos (%s en %s).", successCount, totalCount, sizeText, durText)
			tgHeader = "⚠️ <b>Tarea de Backup Completada [%s]</b>"
		default:
			title = fmt.Sprintf("Tarea completada: %s", summary.ProfileName)
			msg = fmt.Sprintf("%d/%d exitosos (%s en %s).", successCount, totalCount, sizeText, durText)
			tgHeader = "✅ <b>Tarea de Backup Completada [%s]</b>"
		}
	} else {
		switch {
		case failedCount > 0:
			title = fmt.Sprintf("Task finished with errors: %s", summary.ProfileName)
			msg = fmt.Sprintf("%d/%d succeeded, %d failed (%s in %s).", successCount, totalCount, failedCount, sizeText, durText)
			tgHeader = "❌ <b>Backup Task Finished with Errors [%s]</b>"
		case warningCount > 0 || cloudPendingCount > 0:
			title = fmt.Sprintf("Task completed with warnings: %s", summary.ProfileName)
			msg = fmt.Sprintf("%d/%d succeeded (%s in %s).", successCount, totalCount, sizeText, durText)
			tgHeader = "⚠️ <b>Backup Task Completed [%s]</b>"
		default:
			title = fmt.Sprintf("Task completed: %s", summary.ProfileName)
			msg = fmt.Sprintf("%d/%d succeeded (%s in %s).", successCount, totalCount, sizeText, durText)
			tgHeader = "✅ <b>Backup Task Completed [%s]</b>"
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, tgHeader+"\n", html.EscapeString(d.hostname))
	if es {
		fmt.Fprintf(&b, "<b>Perfil:</b> %s (%s)\n", html.EscapeString(summary.ProfileName), html.EscapeString(triggerLabel))
		resLine := fmt.Sprintf("✅ %d/%d exitosos", successCount, totalCount)
		if failedCount > 0 {
			resLine += fmt.Sprintf(" | ❌ %d con error", failedCount)
		}
		fmt.Fprintf(&b, "<b>Resultado:</b> %s\n", resLine)
		fmt.Fprintf(&b, "<b>Tamaño total:</b> %s\n", sizeText)
		fmt.Fprintf(&b, "<b>Duración:</b> %s\n", durText)
		fmt.Fprintf(&b, "<b>Destino:</b> %s", html.EscapeString(destLabel))
	} else {
		fmt.Fprintf(&b, "<b>Profile:</b> %s (%s)\n", html.EscapeString(summary.ProfileName), html.EscapeString(triggerLabel))
		resLine := fmt.Sprintf("✅ %d/%d succeeded", successCount, totalCount)
		if failedCount > 0 {
			resLine += fmt.Sprintf(" | ❌ %d failed", failedCount)
		}
		fmt.Fprintf(&b, "<b>Result:</b> %s\n", resLine)
		fmt.Fprintf(&b, "<b>Total size:</b> %s\n", sizeText)
		fmt.Fprintf(&b, "<b>Duration:</b> %s\n", durText)
		fmt.Fprintf(&b, "<b>Destination:</b> %s", html.EscapeString(destLabel))
	}

	const maxDetailItems = 15
	b.WriteString("\n\n")
	shown := 0
	for _, item := range summary.Items {
		if shown >= maxDetailItems {
			remaining := len(summary.Items) - shown
			if es {
				fmt.Fprintf(&b, "• ... y %d más", remaining)
			} else {
				fmt.Fprintf(&b, "• ... and %d more", remaining)
			}
			break
		}
		shown++
		if !item.Success {
			errTxt := item.Error
			if len(errTxt) > 120 {
				errTxt = errTxt[:120] + "..."
			}
			fmt.Fprintf(&b, "❌ <code>%s</code>: <code>%s</code>\n", html.EscapeString(item.Name), html.EscapeString(errTxt))
		} else if item.Warning != "" || item.CloudPending {
			fmt.Fprintf(&b, "⚠️ <code>%s</code> (%s)\n", html.EscapeString(item.Name), formatBytesHuman(item.SizeBytes))
		} else {
			fmt.Fprintf(&b, "✅ <code>%s</code> (%s)\n", html.EscapeString(item.Name), formatBytesHuman(item.SizeBytes))
		}
	}

	if winWants {
		d.broadcaster.Publish(Event{
			Type:      EventTestNotification,
			Title:     title,
			Message:   msg,
			Severity:  severity,
			ShowToast: true,
		})
	}
	if tgWants {
		d.sendTelegramAsync(strings.TrimSpace(b.String()))
	}
}

func formatBytesHuman(b int64) string {
	const (
		kb = 1024.0
		mb = kb * 1024
		gb = mb * 1024
	)
	fb := float64(b)
	switch {
	case fb >= gb:
		return fmt.Sprintf("%.2f GB", fb/gb)
	case fb >= mb:
		return fmt.Sprintf("%.2f MB", fb/mb)
	case fb >= kb:
		return fmt.Sprintf("%.1f KB", fb/kb)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatDurationShort(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	d = d.Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if m < 60 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	h := m / 60
	m = m % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

func (d *Dispatcher) SendTestAlert(channel string) error {
	if channel == "windows" {
		title := "Prueba PGuardian"
		msg := "Las notificaciones en tiempo real de Windows funcionan correctamente."
		if !d.isSpanish() {
			title = "PGuardian Test"
			msg = "Real-time Windows Toast notifications are working properly."
		}
		d.broadcaster.Publish(Event{
			Type:      EventTestNotification,
			Title:     title,
			Message:   msg,
			Severity:  "info",
			ShowToast: true,
		})
		return nil
	}

	token, err := d.store.GetEncryptedSetting(database.KeyTelegramBotTokenEnc)
	if err != nil || token == "" {
		return fmt.Errorf("telegram bot token not configured")
	}
	recipients, err := d.store.ListTelegramRecipients()
	if err != nil || len(recipients) == 0 {
		return fmt.Errorf("no telegram recipients configured")
	}
	text := fmt.Sprintf("🛡️ <b>PGuardian [%s]</b> — Notificación de prueba exitosa / Test notification successful.", html.EscapeString(d.hostname))
	sent := 0
	var lastErr error
	for _, r := range recipients {
		if !r.Enabled {
			continue
		}
		if err := d.telegram.SendMessage(context.Background(), token, r.ChatID, text); err != nil {
			lastErr = err
		} else {
			sent++
		}
	}
	if sent == 0 && lastErr != nil {
		return lastErr
	}
	return nil
}

func (d *Dispatcher) sendTelegramAsync(htmlMsg string) {
	config.SafeGo("telegram-send", func() {
		token, err := d.store.GetEncryptedSetting(database.KeyTelegramBotTokenEnc)
		if err != nil || token == "" {
			return
		}
		recipients, err := d.store.ListTelegramRecipients()
		if err != nil {
			return
		}
		for _, r := range recipients {
			if !r.Enabled {
				continue
			}
			if err := d.telegram.SendMessage(context.Background(), token, r.ChatID, htmlMsg); err != nil {
				slog.Warn("telegram notification failed", "chatId", r.ChatID, "error", err)
			}
		}
	})
}
