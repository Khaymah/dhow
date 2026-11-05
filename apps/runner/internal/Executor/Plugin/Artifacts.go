package plugin

func (m Manifest) ArtifactNames(with map[string]string) map[string]string {
	out := map[string]string{}
	for name, in := range m.Inputs {
		if v, ok := with[name]; ok && in.IsArtifact() {
			out[name] = v
		}
	}
	return out
}
