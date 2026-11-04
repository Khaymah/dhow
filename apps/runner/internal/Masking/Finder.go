package masking

type Finder struct {
	ac    *automaton
	state int32
}

func (m *Masker) Finder() *Finder { return &Finder{ac: m.current.Load().ac, state: root} }

func (f *Finder) Reset() { f.state = root }

func (f *Finder) Found(p []byte) bool {
	if f.ac.empty() {
		return false
	}
	for _, b := range p {
		f.state = f.ac.step(f.state, b)
		if f.ac.out[f.state] > 0 {
			return true
		}
	}
	return false
}

func (f *Finder) Empty() bool { return f.ac.empty() }
