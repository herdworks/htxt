package bootstrap

import (
	"embed"
)

//go:embed all:website/*
var Website embed.FS
