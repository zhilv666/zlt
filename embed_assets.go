package zhulingtai

import _ "embed"

//go:embed web/index.html
var IndexHTML string

//go:embed ico.ico
var TrayIcon []byte
