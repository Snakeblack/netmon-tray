// Package omarchyplugin embeds the Omarchy shell plugin, so that
// "netmon-tray plugin install" can copy it without the repository. Only the
// files the shell loads are embedded: no tests and no Go source.
package omarchyplugin

import "embed"

//go:embed manifest.json Panel.qml Model.js
var Files embed.FS
