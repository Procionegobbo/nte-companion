// Package web embeds the site's templates and static assets (CSS, JS,
// fonts, the hex-texture SVG) so the final binary is self-contained.
package web

import "embed"

//go:embed templates static
var FS embed.FS
