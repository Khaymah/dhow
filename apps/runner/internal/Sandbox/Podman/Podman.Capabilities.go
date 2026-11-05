package podman

import sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"

func (d *Driver) Capabilities() sandbox.Capabilities {
	return sandbox.Capabilities{Inputs: true, ExactEgress: true}
}
