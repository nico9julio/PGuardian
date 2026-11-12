package winintegration

import (
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

var nonAlphaNum = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

func taskNameForUser(username string) string {
	short := username
	if idx := strings.LastIndex(username, `\`); idx >= 0 {
		short = username[idx+1:]
	}
	safe := nonAlphaNum.ReplaceAllString(short, "_")
	return `\PGuardian\Tray_` + safe
}

// RegisterUserTrayAutostart creates a per-user Windows Scheduled Task triggered strictly at logon of `username`.
// Unselected RDP users have NO scheduled task and therefore execute zero processes on logon.
func RegisterUserTrayAutostart(username, trayExePath string) error {
	var escapedUser, escapedExe strings.Builder
	_ = xml.EscapeText(&escapedUser, []byte(username))
	_ = xml.EscapeText(&escapedExe, []byte(trayExePath))

	taskXML := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>PGuardian Real-Time Notification Agent for %s</Description>
  </RegistrationInfo>
  <Triggers>
    <LogonTrigger>
      <Enabled>true</Enabled>
      <UserId>%s</UserId>
    </LogonTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <UserId>%s</UserId>
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <AllowHardTerminate>true</AllowHardTerminate>
    <StartWhenAvailable>false</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>false</RunOnlyIfNetworkAvailable>
    <AllowStartOnDemand>true</AllowStartOnDemand>
    <Enabled>true</Enabled>
    <Hidden>false</Hidden>
    <ExecutionTimeLimit>PT0S</ExecutionTimeLimit>
    <Priority>7</Priority>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>%s</Command>
    </Exec>
  </Actions>
</Task>`, escapedUser.String(), escapedUser.String(), escapedUser.String(), escapedExe.String())

	tmpFile, err := os.CreateTemp("", "pguardian-task-*.xml")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	_ = tmpFile.Close()
	defer func() { _ = os.Remove(tmpPath) }()

	// Write UTF-16LE with BOM as expected by schtasks.exe
	utf16Bytes := encodeUTF16LEWithBOM(taskXML)
	if err := os.WriteFile(tmpPath, utf16Bytes, 0600); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "schtasks.exe", "/Create", "/TN", taskNameForUser(username), "/XML", filepath.Clean(tmpPath), "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks create failed: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// RemoveUserTrayAutostart deletes the per-user Scheduled Task when notifications are disabled for that user.
func RemoveUserTrayAutostart(username string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "schtasks.exe", "/Delete", "/TN", taskNameForUser(username), "/F")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_, _ = cmd.CombinedOutput()
	return nil
}

func encodeUTF16LEWithBOM(s string) []byte {
	u16, _ := syscall.UTF16FromString(s)
	// Strip trailing null rune
	if len(u16) > 0 && u16[len(u16)-1] == 0 {
		u16 = u16[:len(u16)-1]
	}
	buf := make([]byte, 2+len(u16)*2)
	buf[0] = 0xFF
	buf[1] = 0xFE
	for i, v := range u16 {
		buf[2+i*2] = byte(v)
		buf[2+i*2+1] = byte(v >> 8)
	}
	return buf
}
