package ipc

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"reflect"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"github.com/nico9julio/PGuardian/internal/config"
	"golang.org/x/sys/windows"
)

var (
	modkernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	procGetNamedPipeClientProcID   = modkernel32.NewProc("GetNamedPipeClientProcessId")
)

type CallerIdentity struct {
	PID       uint32
	SessionID uint32
	SID       string
	Username  string
}

// ListenNamedPipe opens the local Windows Named Pipe allowing authenticated local/RDP users to connect.
func ListenNamedPipe() (net.Listener, error) {
	cfg := &winio.PipeConfig{
		// SY = LocalSystem, BA = Administrators, AU = Authenticated local/RDP users (blocks anonymous network access)
		SecurityDescriptor: "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;AU)",
		InputBufferSize:    65536,
		OutputBufferSize:   65536,
	}
	return winio.ListenPipe(config.PipeName, cfg)
}

// DialContext connects to the local PGuardianService Named Pipe.
func DialContext(ctx context.Context) (net.Conn, error) {
	timeout := 4 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl); rem > 0 {
			timeout = rem
		}
	}
	return winio.DialPipe(config.PipeName, &timeout)
}

// NewHTTPClient returns an HTTP client bound to the PGuardian Windows Named Pipe (zero TCP sockets).
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			MaxIdleConns:          4,
			IdleConnTimeout:       30 * time.Second,
			ResponseHeaderTimeout: timeout,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return DialContext(ctx)
			},
		},
	}
}

// InspectCaller extracts the Windows User SID, Username, and RDP Session ID of a Named Pipe client connection.
func InspectCaller(conn net.Conn) CallerIdentity {
	var id CallerIdentity
	h, ok := extractPipeHandle(conn)
	if !ok || h == 0 {
		return id
	}

	var clientPID uint32
	ret, _, _ := procGetNamedPipeClientProcID.Call(uintptr(h), uintptr(unsafe.Pointer(&clientPID)))
	if ret == 0 || clientPID == 0 {
		return id
	}
	id.PID = clientPID
	_ = windows.ProcessIdToSessionId(clientPID, &id.SessionID)

	procHandle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, clientPID)
	if err != nil {
		return id
	}
	defer func() { _ = windows.CloseHandle(procHandle) }()

	var tok windows.Token
	if err := windows.OpenProcessToken(procHandle, windows.TOKEN_QUERY, &tok); err != nil {
		return id
	}
	defer tok.Close()

	tu, err := tok.GetTokenUser()
	if err == nil && tu != nil && tu.User.Sid != nil {
		id.SID = tu.User.Sid.String()
		account, domain, _, lErr := tu.User.Sid.LookupAccount("")
		if lErr == nil {
			if domain != "" {
				id.Username = fmt.Sprintf(`%s\%s`, domain, account)
			} else {
				id.Username = account
			}
		}
	}
	return id
}

func extractPipeHandle(conn net.Conn) (windows.Handle, bool) {
	v := reflect.ValueOf(conn)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return 0, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return 0, false
	}
	hField := v.FieldByName("h")
	if hField.IsValid() && hField.Kind() == reflect.Uintptr {
		return windows.Handle(hField.Uint()), true
	}
	return 0, false
}
