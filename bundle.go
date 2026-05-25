package miseenplace

import "embed"

// Bundled contains the default registry and managed skill payloads shipped with
// the binary. The CLI materializes these into ~/.mise-en-place for normal use.
//
//go:embed registry.yaml skills
var Bundled embed.FS
