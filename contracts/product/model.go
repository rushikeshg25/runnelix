// Package product embeds the versioned, language-neutral product contracts.
package product

import "embed"

//go:embed *.json
var Files embed.FS
