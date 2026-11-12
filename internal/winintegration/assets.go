package winintegration

import _ "embed"

//go:embed assets/tray_idle.ico
var TrayIdleICO []byte

//go:embed assets/tray_running.ico
var TrayRunningICO []byte

//go:embed assets/tray_error.ico
var TrayErrorICO []byte
