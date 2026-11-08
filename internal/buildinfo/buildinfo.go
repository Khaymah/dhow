package buildinfo

import (
	"runtime/debug"
	"time"
)

var (
	version        = "dev"
	commit         = ""
	checkoutImage  = ""
	deploySSHImage = ""
)

func CheckoutImage() string { return checkoutImage }

func DeploySSHImage() string { return deploySSHImage }

type Info struct {
	Version   string    `json:"Version"`
	Commit    string    `json:"Commit"`
	GoVersion string    `json:"GoVersion"`
	BuiltAt   time.Time `json:"BuiltAt,omitzero"`
}

func Get() Info {
	info := Info{Version: version, Commit: commit}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	info.GoVersion = bi.GoVersion
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			if info.Commit == "" {
				info.Commit = s.Value
			}
		case "vcs.time":
			if t, err := time.Parse(time.RFC3339, s.Value); err == nil {
				info.BuiltAt = t.UTC()
			}
		}
	}
	return info
}
