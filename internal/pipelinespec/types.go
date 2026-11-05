package pipelinespec

import (
	"errors"
	"math"

	"go.yaml.in/yaml/v3"
)

type Spec struct {
	Version     int             `yaml:"version"`
	Name        string          `yaml:"name"`
	On          *Triggers       `yaml:"on"`
	Defaults    *Defaults       `yaml:"defaults"`
	Concurrency *Concurrency    `yaml:"concurrency"`
	Jobs        map[string]*Job `yaml:"jobs"`

	positions map[string]Position
}

type Triggers struct {
	Push        *BranchFilter `yaml:"push"`
	Tag         *TagFilter    `yaml:"tag"`
	PullRequest *BranchFilter `yaml:"pull_request"`
	Manual      *Manual       `yaml:"manual"`
	API         *API          `yaml:"api"`
}

type BranchFilter struct {
	Branches []string `yaml:"branches"`
	Paths    []string `yaml:"paths"`
}

type TagFilter struct {
	Patterns []string `yaml:"patterns"`
}

type Manual struct {
	Inputs map[string]*Input `yaml:"inputs"`
}

type API struct{}

type Input struct {
	Type        string   `yaml:"type"`
	Description string   `yaml:"description"`
	Required    bool     `yaml:"required"`
	Default     *Scalar  `yaml:"default"`
	Options     []string `yaml:"options"`
}

type Defaults struct {
	Runner  *Runner `yaml:"runner"`
	Timeout string  `yaml:"timeout"`
}

type Runner struct {
	Labels    []string `yaml:"labels"`
	Isolation string   `yaml:"isolation"`
}

type Concurrency struct {
	Group            string `yaml:"group"`
	CancelInProgress bool   `yaml:"cancel_in_progress"`
}

type Job struct {
	Name        string            `yaml:"name"`
	Needs       []string          `yaml:"needs"`
	If          string            `yaml:"if"`
	Image       string            `yaml:"image"`
	Runner      *Runner           `yaml:"runner"`
	Timeout     string            `yaml:"timeout"`
	Environment string            `yaml:"environment"`
	Strategy    *Strategy         `yaml:"strategy"`
	Network     *Network          `yaml:"network"`
	Resources   *Resources        `yaml:"resources"`
	Env         map[string]Scalar `yaml:"env"`
	Files       map[string]Scalar `yaml:"files"`
	IDToken     *IDToken          `yaml:"id_token"`
	Steps       []*Step           `yaml:"steps"`
	Artifacts   []*Artifact       `yaml:"artifacts"`
	Cache       *Cache            `yaml:"cache"`
}

type Strategy struct {
	Matrix      *Matrix `yaml:"matrix"`
	MaxParallel int     `yaml:"max_parallel"`
}

type Network struct {
	Egress []string `yaml:"egress"`
}

type Resources struct {
	CPU    *Scalar `yaml:"cpu"`
	Memory string  `yaml:"memory"`
	Pids   int     `yaml:"pids"`
	Disk   string  `yaml:"disk"`
}

type Step struct {
	ID      string            `yaml:"id"`
	Name    string            `yaml:"name"`
	If      string            `yaml:"if"`
	Run     string            `yaml:"run"`
	Uses    string            `yaml:"uses"`
	With    map[string]Scalar `yaml:"with"`
	Env     map[string]Scalar `yaml:"env"`
	Files   map[string]Scalar `yaml:"files"`
	Timeout string            `yaml:"timeout"`
}

type IDToken struct {
	Audiences []string `yaml:"audiences"`
}

type Artifact struct {
	Name      string   `yaml:"name"`
	Paths     []string `yaml:"paths"`
	Retention string   `yaml:"retention"`
}

type Cache struct {
	Key   string   `yaml:"key"`
	Paths []string `yaml:"paths"`
}

type Matrix struct {
	Axes    map[string][]Scalar
	Include []map[string]Scalar
	Exclude []map[string]Scalar
}

type ScalarKind int

const (
	KindString ScalarKind = iota + 1
	KindInt
	KindFloat
	KindBool
)

type Scalar struct {
	Kind  ScalarKind
	Text  string
	Int   int64
	Float float64
	Bool  bool
}

var errNotScalar = errors.New("not a plain scalar")

func scalarFromNode(n *yaml.Node) (Scalar, error) {
	if n.Kind != yaml.ScalarNode {
		return Scalar{}, errNotScalar
	}
	s := Scalar{Text: n.Value}
	switch n.Tag {
	case tagStr, tagTimestamp:
		s.Kind = KindString
	case tagInt:
		s.Kind = KindInt
		if err := n.Decode(&s.Int); err != nil {
			return Scalar{}, errNotScalar
		}
	case tagFloat:
		s.Kind = KindFloat
		if err := n.Decode(&s.Float); err != nil || math.IsNaN(s.Float) || math.IsInf(s.Float, 0) {
			return Scalar{}, errNotScalar
		}
	case tagBool:
		s.Kind = KindBool
		if err := n.Decode(&s.Bool); err != nil {
			return Scalar{}, errNotScalar
		}
	default:
		return Scalar{}, errNotScalar
	}
	return s, nil
}

func (s *Scalar) UnmarshalYAML(n *yaml.Node) error {
	v, err := scalarFromNode(n)
	if err != nil {
		return err
	}
	*s = v
	return nil
}

func (s Scalar) Value() any {
	switch s.Kind {
	case KindInt:
		return s.Int
	case KindFloat:
		return s.Float
	case KindBool:
		return s.Bool
	default:
		return s.Text
	}
}

func (s Scalar) String() string {
	return s.Text
}

func (m *Matrix) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind != yaml.MappingNode {
		return errNotScalar
	}
	m.Axes = map[string][]Scalar{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i].Value, n.Content[i+1]
		var err error
		switch key {
		case matrixInclude:
			err = val.Decode(&m.Include)
		case matrixExclude:
			err = val.Decode(&m.Exclude)
		default:
			var values []Scalar
			err = val.Decode(&values)
			m.Axes[key] = values
		}
		if err != nil {
			return err
		}
	}
	return nil
}

const (
	matrixInclude = "include"
	matrixExclude = "exclude"
)
