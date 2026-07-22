package db

import _ "embed"

//go:embed init.sql
var initSQL []byte

func InitSQL() []byte {
	return append([]byte(nil), initSQL...)
}
