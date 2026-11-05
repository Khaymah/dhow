package sandbox_test

import (
	"testing"

	sandbox "github.com/Khaymah/dhow/apps/runner/internal/Sandbox"
)

type bareDriver struct{ sandbox.Driver }

type capableDriver struct{ bareDriver }

func (capableDriver) Capabilities() sandbox.Capabilities {
	return sandbox.Capabilities{Inputs: true, ExactEgress: true}
}

func TestADriverThatDoesNotReportCapabilitiesHasNone(t *testing.T) {
	if got := sandbox.CapabilitiesOf(bareDriver{}); got != (sandbox.Capabilities{}) {
		t.Fatalf("a bare driver reported %+v", got)
	}
	if got := sandbox.CapabilitiesOf(capableDriver{}); !got.Inputs || !got.ExactEgress {
		t.Fatalf("a capable driver reported %+v", got)
	}
}
