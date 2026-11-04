package executor

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	config "github.com/Khaymah/dhow/apps/runner/internal/Config"
)

const (
	dockerHubIndex    = "docker.io"
	dockerHubRegistry = "registry-1.docker.io"
)

var ErrRegistryLogin = errors.New("a registry login from the server is not valid")

type registryLogin struct {
	Username string
	Password string
}

func (a *attempt) openRegistries(values map[string][]byte) error {
	auths := a.grant.GetSpec().GetRegistryAuth()
	if len(auths) == 0 {
		return nil
	}
	a.registries = make(map[string]registryLogin, len(auths))
	for _, ra := range auths {
		raw, ok := values[ra.GetSecretName()]
		if !ok {
			return ErrSecretsMissing
		}
		var l registryLogin
		if err := json.Unmarshal(raw, &l); err != nil || l.Username == "" || l.Password == "" {
			return ErrRegistryLogin
		}
		clear(raw)
		delete(values, ra.GetSecretName())
		a.masker.Add([]byte(l.Password))
		a.scan.Add([]byte(l.Password))
		a.registries[ra.GetHost()] = l
	}
	return nil
}

func imageHost(image string) string {
	host, _, found := strings.Cut(image, "/")
	if !found {
		return ""
	}
	if host == dockerHubIndex {
		return dockerHubRegistry
	}
	return host
}

func (a *attempt) writeAuthFile(seq int, image string) (string, error) {
	host := imageHost(image)
	login, ok := a.registries[host]
	if !ok {
		return "", nil
	}
	entry := map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte(login.Username + ":" + login.Password))}
	auths := map[string]map[string]string{host: entry}
	if host == dockerHubRegistry {
		auths[dockerHubIndex] = entry
	}
	raw, err := json.Marshal(map[string]any{"auths": auths})
	if err != nil {
		return "", err
	}
	defer clear(raw)
	path := filepath.Join(a.dir, "registry-auth-"+strconv.Itoa(seq)+".json")
	if err := os.WriteFile(path, raw, config.PrivateFileMode); err != nil {
		return "", err
	}
	return path, nil
}

func removeAuthFile(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
}

func (a *attempt) needsLogin(image string) bool {
	host := imageHost(image)
	for _, ra := range a.grant.GetSpec().GetRegistryAuth() {
		if ra.GetHost() == host {
			return true
		}
	}
	return false
}
