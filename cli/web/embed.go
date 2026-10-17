// Package web embeds the independently built Jana frontend, adopted at release time.
package web

import "embed"

//go:embed all:static
var Files embed.FS
