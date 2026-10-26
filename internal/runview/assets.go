package runview

import "embed"

// assetFS holds the page shell, its stylesheet and its script, shipped
// inside the binary as templates/templates.go ships the templates.
//
//go:embed assets/page.html assets/viewer.css assets/viewer.js
var assetFS embed.FS
