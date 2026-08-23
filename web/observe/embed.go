package observe

import "embed"

// Dist is the Vite build (index.html + assets). go:embed so iugum observe has a UI.
//
//go:embed all:dist
var Dist embed.FS
