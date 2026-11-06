package policy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	MaxFileBytes = 256 << 10
	MaxImages    = 100

	CodeInvalid        = "COMPOSE_INVALID"
	CodeNotPinned      = "COMPOSE_IMAGE_NOT_PINNED"
	CodeBuild          = "COMPOSE_BUILD_NOT_ALLOWED"
	CodeUnknownService = "COMPOSE_IMAGE_UNKNOWN_SERVICE"
	unsafePrefix       = "COMPOSE_UNSAFE_"
)

var (
	ErrImages = errors.New("the images input is not valid")

	pinned      = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:/-]{0,254}@sha256:[0-9a-f]{64}$`)
	serviceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,63}$`)
)

type Violation struct {
	Code    string
	Service string
	Detail  string
}

func (v *Violation) Error() string {
	out := v.Code
	if v.Service != "" {
		out += " in service " + v.Service
	}
	if v.Detail != "" {
		out += ": " + v.Detail
	}
	return out
}

const OverrideFile = "compose.dhow.yaml"

type Options struct {
	File        string
	DeployPath  string
	AllowUnsafe bool
	Images      map[string]string
	Links       []string
}

type Result struct {
	Images map[string]string
	Binds  []string
	Files  []string
}

func (r Result) Services() []string {
	out := make([]string, 0, len(r.Images))
	for name := range r.Images {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

func ValidImage(ref string) bool { return pinned.MatchString(ref) }

func invalid(format string, args ...any) error {
	return &Violation{Code: CodeInvalid, Detail: fmt.Sprintf(format, args...)}
}

func parse(raw []byte) (map[string]any, error) {
	if len(raw) == 0 || len(raw) > MaxFileBytes {
		return nil, invalid("the file is empty or larger than %d bytes", MaxFileBytes)
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	var doc map[string]any
	if err := dec.Decode(&doc); err != nil || doc == nil {
		return nil, invalid("the file is not a YAML mapping")
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return nil, invalid("the file has more than one YAML document")
	}
	return doc, nil
}

func asMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		out := make(map[string]any, len(m))
		for k, val := range m {
			s, ok := k.(string)
			if !ok {
				return nil, false
			}
			out[s] = val
		}
		return out, true
	}
	return nil, false
}

type checker struct {
	opt   Options
	binds []string
	files []string
}

func Check(raw []byte, opt Options) (Result, error) {
	doc, err := parse(raw)
	if err != nil {
		return Result{}, err
	}
	services, ok := asMap(doc["services"])
	if !ok || len(services) == 0 {
		return Result{}, invalid("the file has no services")
	}
	c := &checker{opt: opt}
	for name := range opt.Images {
		if _, ok := services[name]; !ok {
			return Result{}, &Violation{Code: CodeUnknownService, Service: name}
		}
	}
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	slices.Sort(names)
	res := Result{Images: make(map[string]string, len(names))}
	for _, name := range names {
		if !serviceName.MatchString(name) {
			return Result{}, invalid("a service has a name that is not valid")
		}
		svc, ok := asMap(services[name])
		if !ok {
			return Result{}, invalid("service %s is not a mapping", name)
		}
		image, err := c.image(name, svc)
		if err != nil {
			return Result{}, err
		}
		res.Images[name] = image
	}
	if err := c.always(doc, names, services); err != nil {
		return Result{}, err
	}
	slices.Sort(c.binds)
	slices.Sort(c.files)
	res.Binds, res.Files = c.binds, c.files
	if opt.AllowUnsafe {
		return res, nil
	}
	if err := c.top(doc); err != nil {
		return Result{}, err
	}
	for _, name := range names {
		svc, _ := asMap(services[name])
		if err := c.unsafe(name, svc); err != nil {
			return Result{}, err
		}
	}
	return res, nil
}

func ParseOverride(raw []byte) (map[string]string, error) {
	if len(raw) == 0 || len(raw) > MaxFileBytes {
		return nil, invalid("the override file is empty or too large")
	}
	var doc struct {
		Services map[string]struct {
			Image string `yaml:"image"`
		} `yaml:"services"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&doc); err != nil || len(doc.Services) == 0 {
		return nil, invalid("the override file is not valid")
	}
	out := make(map[string]string, len(doc.Services))
	for name, svc := range doc.Services {
		if !serviceName.MatchString(name) || !pinned.MatchString(svc.Image) {
			return nil, &Violation{Code: CodeNotPinned, Service: name}
		}
		out[name] = svc.Image
	}
	return out, nil
}

func (c *checker) image(name string, svc map[string]any) (string, error) {
	if _, ok := svc["build"]; ok {
		return "", &Violation{Code: CodeBuild, Service: name}
	}
	ref, overridden := c.opt.Images[name]
	if !overridden {
		ref, _ = svc["image"].(string)
	}
	if !pinned.MatchString(ref) {
		return "", &Violation{Code: CodeNotPinned, Service: name}
	}
	return ref, nil
}

func ParseImages(text string) (map[string]string, error) {
	out := map[string]string{}
	for _, item := range strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == '\r' }) {
		name, ref, ok := strings.Cut(item, "=")
		if !ok || !serviceName.MatchString(name) || !pinned.MatchString(ref) {
			return nil, fmt.Errorf("%w: every item is service=image@sha256:digest", ErrImages)
		}
		if _, dup := out[name]; dup {
			return nil, fmt.Errorf("%w: the service %s is named twice", ErrImages, name)
		}
		out[name] = ref
		if len(out) > MaxImages {
			return nil, fmt.Errorf("%w: more than %d images", ErrImages, MaxImages)
		}
	}
	return out, nil
}

func Override(images map[string]string) ([]byte, error) {
	type service struct {
		Image string `yaml:"image"`
	}
	doc := struct {
		Services map[string]service `yaml:"services"`
	}{Services: make(map[string]service, len(images))}
	for name, ref := range images {
		if !serviceName.MatchString(name) || !pinned.MatchString(ref) {
			return nil, ErrImages
		}
		doc.Services[name] = service{Image: ref}
	}
	return yaml.Marshal(doc)
}
