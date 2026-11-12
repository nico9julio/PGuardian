package notify

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"os"

	"github.com/nico9julio/PGuardian/internal/config"
	"github.com/nico9julio/PGuardian/internal/database"
)

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
	rules := d.ruleMap()
	winRule := rules["windows"]
	tgRule := rules["telegram"]

	sizeMB := float64(sizeBytes) / (1024 * 1024)
	var title, msg, tgMsg string
	if d.isSpanish() {
		title = "Copia completada: " + dbName
		if cloudUploaded {
			msg = fmt.Sprintf("Perfil '%s' completado (%.2f MB) en Local y Google Drive.", profileName, sizeMB)
		} else {
			msg = fmt.Sprintf("Perfil '%s' completado localmente (%.2f MB).", profileName, sizeMB)
		}
		tgMsg = fmt.Sprintf("✅ <b>PGuardian [%s]</b>\n<b>Base:</b> <code>%s</code>\n<b>Perfil:</b> %s\n<b>Tamaño:</b> %.2f MB\n<b>Destino:</b> %s",
			html.EscapeString(d.hostname), html.EscapeString(dbName), html.EscapeString(profileName), sizeMB,
			map[bool]string{true: "Local + Google Drive", false: "Local"}[cloudUploaded])
	} else {
		title = "Backup completed: " + dbName
		if cloudUploaded {
			msg = fmt.Sprintf("Profile '%s' completed (%.2f MB) on Local & Google Drive.", profileName, sizeMB)
		} else {
			msg = fmt.Sprintf("Profile '%s' completed locally (%.2f MB).", profileName, sizeMB)
		}
		tgMsg = fmt.Sprintf("✅ <b>PGuardian [%s]</b>\n<b>Database:</b> <code>%s</code>\n<b>Profile:</b> %s\n<b>Size:</b> %.2f MB\n<b>Target:</b> %s",
			html.EscapeString(d.hostname), html.EscapeString(dbName), html.EscapeString(profileName), sizeMB,
			map[bool]string{true: "Local + Google Drive", false: "Local"}[cloudUploaded])
	}

	d.broadcaster.Publish(Event{
		Type:        EventBackupCompleted,
		Title:       title,
		Message:     msg,
		Severity:    "success",
		ShowToast:   winRule.OnSuccess,
		ExecutionID: execID,
		DBName:      dbName,
	})

	if tgRule.OnSuccess {
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
		msg = fmt.Sprintf("24h: %d exitosas, %d fallidas, %d pendientes nube.", metrics.Success24h, metrics.Failed24h, metrics.PendingCloudCount)
		tgMsg = fmt.Sprintf("📊 <b>Resumen Diario PGuardian [%s]</b>\n• Copias 24h: <b>%d</b> (✅ %d | ❌ %d)\n• Cola Google Drive: <b>%d</b>\n• Espacio local usado: <b>%.2f GB</b>",
			html.EscapeString(d.hostname), metrics.TotalExecutions24h, metrics.Success24h, metrics.Failed24h, metrics.PendingCloudCount, totalGB)
	} else {
		title = "PGuardian Daily Summary"
		msg = fmt.Sprintf("24h: %d succeeded, %d failed, %d cloud pending.", metrics.Success24h, metrics.Failed24h, metrics.PendingCloudCount)
		tgMsg = fmt.Sprintf("📊 <b>PGuardian Daily Summary [%s]</b>\n• Backups 24h: <b>%d</b> (✅ %d | ❌ %d)\n• Drive Queue: <b>%d</b>\n• Local storage used: <b>%.2f GB</b>",
			html.EscapeString(d.hostname), metrics.TotalExecutions24h, metrics.Success24h, metrics.Failed24h, metrics.PendingCloudCount, totalGB)
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
