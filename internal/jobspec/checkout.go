package jobspec

import (
	"regexp"
	"strings"

	"github.com/Khaymah/dhow/internal/gitref"
	"github.com/Khaymah/dhow/internal/gitsource"
	runnerv1 "github.com/Khaymah/dhow/packages/proto/gen/go/dhow/runner/v1"
)

const (
	CheckoutSecret       = "dhow.git.checkout" //nolint:gosec // The reserved name of the checkout secret, not its value.
	CheckoutSecretFile   = "git-credential"    //nolint:gosec // The file name the checkout secret gets, not its value.
	DefaultCheckoutDepth = 1
	MaxPluginInputs      = 32
	MaxPluginInputBytes  = 32 << 10
	MaxStepIDLength      = 63
	FeatureCheckout      = "checkout"
	FeaturePlugins       = "plugins"
	PluginInputEnvPrefix = "DHOW_INPUT_"
)

var (
	stepIDPattern      = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)
	pluginInputPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

func Features(spec *runnerv1.JobSpec) []string {
	var out []string
	if spec.GetCheckout() != nil {
		out = append(out, FeatureCheckout)
	}
	for _, s := range spec.GetSteps() {
		if s.GetPlugin() != nil {
			out = append(out, FeaturePlugins)
			break
		}
	}
	if len(spec.GetArtifacts()) > 0 || len(spec.GetInputs()) > 0 {
		out = append(out, FeatureArtifacts)
	}
	if spec.GetCache() != nil {
		out = append(out, FeatureCache)
	}
	return out
}

func HasRunStep(spec *runnerv1.JobSpec) bool {
	for _, s := range spec.GetSteps() {
		if s.GetPlugin() == nil {
			return true
		}
	}
	return false
}

func normalizeCheckout(c *runnerv1.Checkout) {
	if c == nil {
		return
	}
	if c.GetDepth() == 0 {
		c.Depth = DefaultCheckoutDepth
	}
}

func (c *checker) stepBody(field string, s *runnerv1.Step) {
	if s.GetPlugin() != nil {
		if s.GetRun() != "" {
			c.add(field+".Run", ReasonInvalid)
		}
		c.plugin(field+".Plugin", s.GetPlugin())
		return
	}
	switch {
	case strings.TrimSpace(s.GetRun()) == "":
		c.add(field+".Run", ReasonRequired)
	case len(s.GetRun()) > MaxRunBytes:
		c.add(field+".Run", ReasonTooLong)
	case strings.ContainsRune(s.GetRun(), 0):
		c.add(field+".Run", ReasonInvalid)
	}
}

func (c *checker) stepID(field string, id string, seen map[string]bool) {
	if id == "" {
		return
	}
	switch {
	case !stepIDPattern.MatchString(id):
		c.add(field+".Id", ReasonInvalid)
	case seen[id]:
		c.add(field+".Id", ReasonDuplicate)
	}
	seen[id] = true
}

func (c *checker) plugin(field string, p *runnerv1.Plugin) {
	switch {
	case p.GetImage() == "":
		c.add(field+".Image", ReasonRequired)
	case len(p.GetImage()) > MaxImageLength:
		c.add(field+".Image", ReasonTooLong)
	case !imagePattern.MatchString(p.GetImage()):
		if imageAnyRef.MatchString(p.GetImage()) {
			c.add(field+".Image", ReasonNotPinned)
		} else {
			c.add(field+".Image", ReasonInvalid)
		}
	}
	if len(p.GetInputs()) > MaxPluginInputs {
		c.add(field+".Inputs", ReasonTooMany)
		return
	}
	for _, name := range sortedKeys(p.GetInputs()) {
		v := p.GetInputs()[name]
		switch {
		case !pluginInputPattern.MatchString(name):
			c.add(field+".Inputs."+name, ReasonInvalid)
		case len(v) > MaxPluginInputBytes:
			c.add(field+".Inputs."+name, ReasonTooLong)
		case strings.ContainsRune(v, 0):
			c.add(field+".Inputs."+name, ReasonInvalid)
		}
	}
}

func (c *checker) checkout(spec *runnerv1.JobSpec) {
	co := spec.GetCheckout()
	if co == nil {
		for i, s := range spec.GetSecrets() {
			if s.GetName() == CheckoutSecret {
				c.add(at("Secrets", i)+".Name", ReasonReserved)
			}
		}
		return
	}
	u, err := gitsource.Parse(co.GetRepositoryUrl())
	switch {
	case err != nil:
		c.add("Checkout.RepositoryUrl", ReasonInvalid)
	case u.String() != co.GetRepositoryUrl():
		c.add("Checkout.RepositoryUrl", ReasonInvalid)
	case transportOf(u) != co.GetTransport():
		c.add("Checkout.Transport", ReasonInvalid)
	}
	if _, err := gitref.Parse(co.GetRef()); err != nil {
		c.add("Checkout.Ref", ReasonInvalid)
	}
	if err := gitref.CheckSha(co.GetSha()); err != nil {
		c.add("Checkout.Sha", ReasonInvalid)
	}
	if d := co.GetDepth(); d < 1 || d > gitsource.MaxDepth {
		c.add("Checkout.Depth", ReasonOutOfRange)
	}
	if err == nil {
		c.checkoutAccess(spec, u)
	}
}

func transportOf(u gitsource.URL) runnerv1.CheckoutTransport {
	switch u.Transport {
	case gitsource.TransportHTTPS:
		return runnerv1.CheckoutTransport_CHECKOUT_TRANSPORT_HTTPS
	case gitsource.TransportSSH:
		return runnerv1.CheckoutTransport_CHECKOUT_TRANSPORT_SSH
	default:
		return runnerv1.CheckoutTransport_CHECKOUT_TRANSPORT_UNSPECIFIED
	}
}

func (c *checker) checkoutAccess(spec *runnerv1.JobSpec, u gitsource.URL) {
	co := spec.GetCheckout()
	ssh := u.Transport == gitsource.TransportSSH
	switch {
	case ssh && gitsource.CheckKnownHost(u, co.GetKnownHost()) != nil:
		c.add("Checkout.KnownHost", ReasonInvalid)
	case !ssh && co.GetKnownHost() != "":
		c.add("Checkout.KnownHost", ReasonInvalid)
	}
	switch co.GetCredentialSecret() {
	case "":
		if ssh {
			c.add("Checkout.CredentialSecret", ReasonRequired)
		}
	case CheckoutSecret:
	default:
		c.add("Checkout.CredentialSecret", ReasonInvalid)
	}
	found := false
	for i, s := range spec.GetSecrets() {
		if s.GetName() != CheckoutSecret {
			continue
		}
		field := at("Secrets", i)
		if co.GetCredentialSecret() != CheckoutSecret || found {
			c.add(field+".Name", ReasonReserved)
		}
		found = true
		if s.GetAsFile() != CheckoutSecretFile || len(s.GetStepIndexes()) > 0 {
			c.add(field, ReasonReserved)
		}
	}
	if co.GetCredentialSecret() == CheckoutSecret && !found {
		c.add("Checkout.CredentialSecret", ReasonRequired)
	}
}

func (c *checker) pluginSecret(field, name string, s *runnerv1.SecretRef, spec *runnerv1.JobSpec) {
	idx := s.GetStepIndexes()
	if len(idx) != 1 || idx[0] < 0 || int(idx[0]) >= len(spec.GetSteps()) {
		c.add(field+".AsEnv", ReasonReserved)
		return
	}
	p := spec.GetSteps()[idx[0]].GetPlugin()
	input := strings.ToLower(strings.TrimPrefix(name, PluginInputEnvPrefix))
	if p == nil || strings.ToUpper(input) != strings.TrimPrefix(name, PluginInputEnvPrefix) || !pluginInputPattern.MatchString(input) {
		c.add(field+".AsEnv", ReasonReserved)
		return
	}
	if _, set := p.GetInputs()[input]; set {
		c.add(field+".AsEnv", ReasonDuplicate)
	}
}
