package buildinfo

import "runtime/debug"

var (
	version       = "dev"
	commit        = ""
	checkoutImage = ""
)

func CheckoutImage() string { return checkoutImage }

type Info struct {
	Version   string `json:"Version"`
	Commit    string `json:"Commit"`
	GoVersion string `json:"GoVersion"`
}

func Get() Info {
	info := Info{Version: version, Commit: commit}
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return info
	}
	info.GoVersion = bi.GoVersion
	if info.Commit != "" {
		return info
	}
	for _, s := range bi.Settings {
		if s.Key == "vcs.revision" {
			info.Commit = s.Value
		}
	}
	return info
}
