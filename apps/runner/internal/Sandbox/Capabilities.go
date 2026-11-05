package sandbox

type Capabilities struct {
	Inputs      bool
	ExactEgress bool
}

type CapabilityReporter interface {
	Capabilities() Capabilities
}

func CapabilitiesOf(d Driver) Capabilities {
	if r, ok := d.(CapabilityReporter); ok {
		return r.Capabilities()
	}
	return Capabilities{}
}
