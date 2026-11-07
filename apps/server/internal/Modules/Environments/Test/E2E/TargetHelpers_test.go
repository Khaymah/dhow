package e2e_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"syscall"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	apptest "github.com/Khaymah/dhow/apps/server/internal/App/Testing"
)

const (
	targetPath   = "/srv/shop"
	publicHost   = "deploy.example.test"
	privateHost  = "private.example.test"
	metadataHost = "meta.example.test"
)

type hostRow struct {
	Id                        string
	Position, Port            int
	Host                      string
	KeyPinned                 bool
	HostKeyFingerprint        *string
	PendingHostKeyFingerprint *string
	HostKeyConfirmedAt        *string
	HostKeyConfirmedByUserId  *string
}

type settingsView struct {
	Rolling   *struct{ BatchSize, BatchPauseSeconds int }
	AllowRoot *bool
	Ssh       *struct {
		ReloadCommand, PreSwitchCommand, PostSwitchCommand string
		ReloadCommandSet, PreSwitchCommandSet              bool
		PostSwitchCommandSet                               bool
		Shared                                             []string
	}
	Compose *struct {
		ProjectName, ComposeFile string
		AllowUnsafeCompose       bool
	}
}

type targetView struct {
	Id, EnvironmentId, Name, Kind, Username, DeployPath string
	KeySecretName, PublicKey                            *string
	KeepReleases                                        int
	Settings                                            settingsView
	AllowedImages                                       []string
	Hosts                                               []hostRow
	Ready                                               bool
	NotReady                                            []string
	Version                                             int64
}

func lookup(_ context.Context, host string) ([]netip.Addr, error) {
	answers := map[string]string{
		publicHost:   "203.0.113.20",
		privateHost:  "10.1.2.3",
		metadataHost: "169.254.169.254",
	}
	addr, ok := answers[host]
	if !ok {
		return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
	}
	return []netip.Addr{netip.MustParseAddr(addr)}, nil
}

func toLoopback(ctx context.Context, network, address string) (net.Conn, error) {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	if port == "22" {
		return nil, &net.OpError{Op: "dial", Net: network, Err: syscall.ECONNREFUSED}
	}
	var d net.Dialer
	return d.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
}

type sshServer struct {
	port        string
	fingerprint string
	authorized  string
}

func newHostKey(t *testing.T) ssh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	signer, err := ssh.NewSignerFromKey(priv)
	require.NoError(t, err)
	return signer
}

func describe(signer ssh.Signer, port string) sshServer {
	return sshServer{
		port: port, fingerprint: ssh.FingerprintSHA256(signer.PublicKey()),
		authorized: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey()))),
	}
}

func startSSH(t *testing.T) sshServer {
	t.Helper()
	signer := newHostKey(t)
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				_, _, _, _ = ssh.NewServerConn(conn, cfg)
			}()
		}
	}()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	require.NoError(t, err)
	return describe(signer, port)
}

func newTargetWorld(t *testing.T) world {
	t.Helper()
	env := apptest.NewWithOptions(t, apptest.Options{GitLookup: lookup, GitDial: toLoopback})
	owner, d := env.SetupAndLogin()
	org := env.OrganizationID(d.OrgSlug)
	ownerID := uuid.MustParse(owner.UserID)
	return world{env: env, owner: owner, ownerID: ownerID, password: d.Password, org: org, project: env.SeedProject(org, ownerID)}
}

func (w world) targetsPath(envName, rest string) string {
	return w.path(w.project, "/"+envName+"/targets"+rest)
}

func (w world) targetBody(name string, hosts ...map[string]any) map[string]any {
	if len(hosts) == 0 {
		hosts = []map[string]any{{"Host": publicHost, "Port": 2201}}
	}
	return map[string]any{
		"Name": name, "Kind": "ssh", "Username": "deploy", "DeployPath": targetPath, "Hosts": hosts, "CurrentFactor": w.ownerFactor(),
	}
}

func (w world) createTarget(c *apptest.Client, envName string, body map[string]any) apptest.Response {
	return c.Do(apptest.Request{Method: http.MethodPost, Path: w.targetsPath(envName, ""), Body: body})
}

func (w world) targetCall(c *apptest.Client, method, envName, rest string, body any) apptest.Response {
	return c.Do(apptest.Request{Method: method, Path: w.targetsPath(envName, rest), Body: body})
}

func decodeTarget(t *testing.T, res apptest.Response, status int) targetView {
	t.Helper()
	require.Equal(t, status, res.Status, string(res.Raw))
	var v targetView
	res.Decode(t, &v)
	return v
}

func (w world) newTarget(t *testing.T, envName, name string, hosts ...map[string]any) targetView {
	t.Helper()
	return decodeTarget(t, w.createTarget(w.owner, envName, w.targetBody(name, hosts...)), http.StatusCreated)
}

func hostEntry(host string, port int) map[string]any {
	return map[string]any{"Host": host, "Port": port}
}

func (w world) fetchKey(c *apptest.Client, envName, target, hostID string) apptest.Response {
	return w.targetCall(c, http.MethodPost, envName, "/"+target+"/hosts/"+hostID+"/fetch-host-key", nil)
}

func (w world) confirmKey(c *apptest.Client, envName, target, hostID, fingerprint string) apptest.Response {
	return w.targetCall(c, http.MethodPost, envName, "/"+target+"/hosts/"+hostID+"/confirm-host-key",
		map[string]any{"Fingerprint": fingerprint, "CurrentFactor": w.ownerFactor()})
}
