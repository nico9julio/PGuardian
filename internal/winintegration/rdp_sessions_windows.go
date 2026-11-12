package winintegration

import (
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modwtsapi32              = windows.NewLazySystemDLL("wtsapi32.dll")
	modnetapi32              = windows.NewLazySystemDLL("netapi32.dll")
	moduserenv               = windows.NewLazySystemDLL("userenv.dll")
	procWTSEnumerateSessions = modwtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSQuerySessionInfo  = modwtsapi32.NewProc("WTSQuerySessionInformationW")
	procWTSFreeMemory        = modwtsapi32.NewProc("WTSFreeMemory")
	procWTSQueryUserToken    = modwtsapi32.NewProc("WTSQueryUserToken")
	procNetUserEnum          = modnetapi32.NewProc("NetUserEnum")
	procNetApiBufferFree     = modnetapi32.NewProc("NetApiBufferFree")
	procCreateEnvBlock       = moduserenv.NewProc("CreateEnvironmentBlock")
	procDestroyEnvBlock      = moduserenv.NewProc("DestroyEnvironmentBlock")
)

const (
	wtsCurrentServerHandle = 0
	wtsUserName            = 5
	wtsDomainName          = 7
	wtsConnectState        = 8
	wtsWinStationName      = 6

	wtsActive       = 0
	wtsConnected    = 1
	wtsDisconnected = 4

	filterNormalAccount = 0x0002
	ufAccountDisable    = 0x0002
)

type wtsSessionInfo struct {
	SessionID      uint32
	WinStationName *uint16
	State          uint32
}

type userInfo1 struct {
	Name        *uint16
	Password    *uint16
	PasswordAge uint32
	Priv        uint32
	HomeDir     *uint16
	Comment     *uint16
	Flags       uint32
	ScriptPath  *uint16
}

type ServerUserSession struct {
	Username       string `json:"username"`       // e.g. "SERVER\Nico" or "Nico"
	ShortName      string `json:"shortName"`      // e.g. "Nico"
	SID            string `json:"sid"`
	SessionID      uint32 `json:"sessionId"`
	SessionType    string `json:"sessionType"`    // "RDP" | "Console" | "Offline"
	ConnectionStat string `json:"connectionStat"` // "Active" | "Disconnected" | "Offline"
	NotifyEnabled  bool   `json:"notifyEnabled"`
}

// ListActiveSessions enumerates all interactive Console and RDP sessions on this Windows machine.
func ListActiveSessions() ([]ServerUserSession, error) {
	var pSessionInfo *wtsSessionInfo
	var count uint32
	ret, _, err := procWTSEnumerateSessions.Call(
		wtsCurrentServerHandle,
		0,
		1,
		uintptr(unsafe.Pointer(&pSessionInfo)),
		uintptr(unsafe.Pointer(&count)),
	)
	if ret == 0 {
		return nil, fmt.Errorf("WTSEnumerateSessionsW failed: %w", err)
	}
	defer func() {
		if pSessionInfo != nil {
			_, _, _ = procWTSFreeMemory.Call(uintptr(unsafe.Pointer(pSessionInfo)))
		}
	}()

	sessions := unsafe.Slice(pSessionInfo, count)
	var out []ServerUserSession
	for _, s := range sessions {
		if s.SessionID == 0 {
			continue // Session 0 is Services
		}
		user := queryWTSString(s.SessionID, wtsUserName)
		if user == "" {
			continue
		}
		domain := queryWTSString(s.SessionID, wtsDomainName)
		station := windows.UTF16PtrToString(s.WinStationName)

		fullUser := user
		if domain != "" {
			fullUser = domain + `\` + user
		}
		sidStr := LookupSID(fullUser)

		sessType := "Console"
		if strings.HasPrefix(strings.ToUpper(station), "RDP") {
			sessType = "RDP"
		}
		stateStr := "Offline"
		switch s.State {
		case wtsActive, wtsConnected:
			stateStr = "Active"
		case wtsDisconnected:
			stateStr = "Disconnected"
		}

		out = append(out, ServerUserSession{
			Username:       fullUser,
			ShortName:      user,
			SID:            sidStr,
			SessionID:      s.SessionID,
			SessionType:    sessType,
			ConnectionStat: stateStr,
		})
	}
	return out, nil
}

// ListAllServerUsers merges local enabled Windows user accounts with currently active RDP/domain sessions.
func ListAllServerUsers(enabledMap map[string]bool) ([]ServerUserSession, error) {
	active, _ := ListActiveSessions()
	byLower := make(map[string]ServerUserSession)

	hostname, _ := windows.ComputerName()
	localUsers := listLocalUserAccounts()
	for _, u := range localUsers {
		full := u
		if hostname != "" {
			full = hostname + `\` + u
		}
		key := strings.ToLower(u)
		byLower[key] = ServerUserSession{
			Username:       full,
			ShortName:      u,
			SID:            LookupSID(full),
			SessionType:    "Offline",
			ConnectionStat: "Offline",
			NotifyEnabled:  enabledMap[strings.ToLower(full)] || enabledMap[key],
		}
	}

	for _, s := range active {
		key := strings.ToLower(s.ShortName)
		s.NotifyEnabled = enabledMap[strings.ToLower(s.Username)] || enabledMap[key]
		byLower[key] = s
	}

	out := make([]ServerUserSession, 0, len(byLower))
	for _, v := range byLower {
		out = append(out, v)
	}
	return out, nil
}

func listLocalUserAccounts() []string {
	var bufPtr *userInfo1
	var entriesRead, totalEntries, resumeHandle uint32
	ret, _, _ := procNetUserEnum.Call(
		0,
		1, // USER_INFO_1
		filterNormalAccount,
		uintptr(unsafe.Pointer(&bufPtr)),
		0xFFFFFFFF,
		uintptr(unsafe.Pointer(&entriesRead)),
		uintptr(unsafe.Pointer(&totalEntries)),
		uintptr(unsafe.Pointer(&resumeHandle)),
	)
	if bufPtr != nil {
		defer func() { _, _, _ = procNetApiBufferFree.Call(uintptr(unsafe.Pointer(bufPtr))) }()
	}
	if ret != 0 || entriesRead == 0 || bufPtr == nil {
		return nil
	}

	items := unsafe.Slice(bufPtr, entriesRead)
	var names []string
	for _, item := range items {
		if item.Flags&ufAccountDisable != 0 {
			continue
		}
		name := windows.UTF16PtrToString(item.Name)
		if name == "" || strings.EqualFold(name, "DefaultAccount") || strings.EqualFold(name, "WDAGUtilityAccount") || strings.EqualFold(name, "Guest") || strings.EqualFold(name, "Invitado") {
			continue
		}
		names = append(names, name)
	}
	return names
}

func queryWTSString(sessionID uint32, infoClass uint32) string {
	var buf *uint16
	var bytesReturned uint32
	ret, _, _ := procWTSQuerySessionInfo.Call(
		wtsCurrentServerHandle,
		uintptr(sessionID),
		uintptr(infoClass),
		uintptr(unsafe.Pointer(&buf)),
		uintptr(unsafe.Pointer(&bytesReturned)),
	)
	if ret == 0 || buf == nil {
		return ""
	}
	defer func() { _, _, _ = procWTSFreeMemory.Call(uintptr(unsafe.Pointer(buf))) }()
	return windows.UTF16PtrToString(buf)
}

func LookupSID(accountName string) string {
	sid, _, _, err := windows.LookupSID("", accountName)
	if err != nil || sid == nil {
		return ""
	}
	return sid.String()
}

// LaunchProcessInSession starts an executable inside an active user's Console or RDP session from Session 0 (LocalSystem).
func LaunchProcessInSession(sessionID uint32, exePath string, args string) error {
	if sessionID == 0 {
		return fmt.Errorf("cannot launch UI process in Session 0")
	}
	var userToken windows.Token
	ret, _, err := procWTSQueryUserToken.Call(uintptr(sessionID), uintptr(unsafe.Pointer(&userToken)))
	if ret == 0 {
		return fmt.Errorf("WTSQueryUserToken(session=%d): %w", sessionID, err)
	}
	defer userToken.Close()

	var primaryToken windows.Token
	err = windows.DuplicateTokenEx(
		userToken,
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityIdentification,
		windows.TokenPrimary,
		&primaryToken,
	)
	if err != nil {
		return fmt.Errorf("DuplicateTokenEx: %w", err)
	}
	defer primaryToken.Close()

	var envBlock *uint16
	ret, _, _ = procCreateEnvBlock.Call(uintptr(unsafe.Pointer(&envBlock)), uintptr(primaryToken), 0)
	if ret != 0 && envBlock != nil {
		defer func() { _, _, _ = procDestroyEnvBlock.Call(uintptr(unsafe.Pointer(envBlock))) }()
	}

	cmdLine := fmt.Sprintf(`"%s"`, exePath)
	if strings.TrimSpace(args) != "" {
		cmdLine += " " + args
	}
	cmdLineUTF16, err := syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		return err
	}
	desktopUTF16, _ := syscall.UTF16PtrFromString(`winsta0\default`)

	var si windows.StartupInfo
	si.Cb = uint32(unsafe.Sizeof(si))
	si.Desktop = desktopUTF16

	var pi windows.ProcessInformation
	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NO_WINDOW)
	err = windows.CreateProcessAsUser(
		primaryToken,
		nil,
		cmdLineUTF16,
		nil,
		nil,
		false,
		flags,
		envBlock,
		nil,
		&si,
		&pi,
	)
	if err != nil {
		return fmt.Errorf("CreateProcessAsUser: %w", err)
	}
	_ = windows.CloseHandle(pi.Thread)
	_ = windows.CloseHandle(pi.Process)
	return nil
}
