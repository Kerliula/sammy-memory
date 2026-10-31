// Package db embeds Sammy's goose migrations into the binary.
package db

import "embed"

// Migrations holds every goose migration under migrations/.
//
//go:embed migrations/*.sql
var Migrations embed.FS
