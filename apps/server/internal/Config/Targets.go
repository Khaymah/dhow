package config

import "time"

const (
	TargetDialPerUser  = 30
	TargetDialPerOrg   = 120
	TargetDialWindow   = time.Hour
	TargetKeyComment   = "dhow-deploy-target"
	TargetKeySuffix    = "_SSH_KEY"
	TargetKeySealLabel = "target-key"
)
