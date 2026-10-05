// Package dbschema embeds the SQL migrations so the server binary can apply them.
package dbschema

import "embed"

//go:embed migrations/*.sql
var Migrations embed.FS
