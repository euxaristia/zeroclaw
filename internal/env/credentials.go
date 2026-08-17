// Credential store capture for the web UI. The agent's zero credential store
// lives in the container at ~/.config/zero/credentials.enc, encrypted under a
// per-user 32-byte secret in credentials.enc.secret. This file reads and
// writes that store with the same on-disk format zero's own credstore and
// securefile packages use (AES-256-GCM, nonce || ciphertext, JSON map of
// provider name to key) so the two never disagree about what is stored.
//
// Zero owns provider knowledge (base URLs, default models), so a provider
// profile that does not exist yet is scaffolded through `zero providers add`
// rather than by duplicating zero's catalog here. Existing profiles are left
// untouched so a synced or customized provider is never clobbered.
package env

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const (
	zeroConfigDir        = Home + "/.config/zero"
	credentialSecretName = "credentials.enc.secret"
	credentialStoreName  = "credentials.enc"
	// credentialSecretSize is the AES-256 key length zero keeps in the secret
	// file.
	credentialSecretSize = 32
)

// credentialPaths returns the container paths of zero's credential store and
// its secret.
func credentialPaths() (secretPath, storePath string) {
	return zeroConfigDir + "/" + credentialSecretName, zeroConfigDir + "/" + credentialStoreName
}

// readContainerFile reads a file out of the container, reporting exists=false
// when the file is absent. Absence is told apart from a real read error by a
// distinct exit code, so callers can distinguish "no file yet" from "broken".
func readContainerFile(container, containerPath string) ([]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := DockerCommandContext(ctx, "exec", container, "sh", "-c",
		`p="$1"; if [ -e "$p" ]; then cat "$p"; exit 0; else exit 2; fi`, "sh", containerPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("reading %s in %s: %w", containerPath, container, err)
	}
	return out, true, nil
}

// writeContainerFile writes data to a container path owned by the zeroclaw
// user, via a host-side temp file and docker cp. This is the same mechanism
// SyncAuth uses for the one-time credential adoption; the payloads here are
// tiny (a JSON config and a credential store), so the round trip is cheap.
func writeContainerFile(container, containerPath string, data []byte) error {
	tmp, err := os.CreateTemp("", "zeroclaw-cred-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if _, err := docker("cp", tmpPath, container+":"+containerPath); err != nil {
		return err
	}
	if _, err := docker("exec", "-u", "root", container, "chown", "zeroclaw:zeroclaw", containerPath); err != nil {
		return err
	}
	return nil
}

// newCredentialGCM builds the AES-GCM AEAD zero's securefile uses.
func newCredentialGCM(secret []byte) (cipher.AEAD, error) {
	if len(secret) != credentialSecretSize {
		return nil, fmt.Errorf("credential secret is %d bytes, want %d", len(secret), credentialSecretSize)
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// sealCredentialStore encrypts the provider->key map the way zero's
// securefile does: JSON encode, then AES-256-GCM with a fresh nonce prefixed,
// so the blob is nonce || ciphertext.
func sealCredentialStore(secret []byte, data map[string]string) ([]byte, error) {
	payload, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	gcm, err := newCredentialGCM(secret)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, payload, nil), nil
}

// openCredentialStore decrypts a nonce || ciphertext blob back into the
// provider->key map, failing closed on a wrong secret or tampered file.
func openCredentialStore(secret, blob []byte) (map[string]string, error) {
	gcm, err := newCredentialGCM(secret)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, errors.New("credential store is too short")
	}
	plain, err := gcm.Open(nil, blob[:gcm.NonceSize()], blob[gcm.NonceSize():], nil)
	if err != nil {
		return nil, fmt.Errorf("credential store decrypt (wrong secret or tampered): %w", err)
	}
	data := map[string]string{}
	if len(plain) == 0 {
		return data, nil
	}
	if err := json.Unmarshal(plain, &data); err != nil {
		return nil, fmt.Errorf("credential store parse: %w", err)
	}
	return data, nil
}

// loadCredentialStore reads and decrypts the agent's zero credential store. A
// missing secret is generated in memory (it is persisted only on save); a
// missing store means an empty map. secretExisted tells saveCredentialStore
// whether the secret file still needs to be written.
func loadCredentialStore(container string) (secret []byte, secretExisted bool, data map[string]string, err error) {
	secretPath, storePath := credentialPaths()
	secret, secretExisted, err = readContainerFile(container, secretPath)
	if err != nil {
		return nil, false, nil, err
	}
	if !secretExisted {
		secret = make([]byte, credentialSecretSize)
		if _, err := io.ReadFull(rand.Reader, secret); err != nil {
			return nil, false, nil, err
		}
	}
	blob, storeExists, err := readContainerFile(container, storePath)
	if err != nil {
		return nil, false, nil, err
	}
	data = map[string]string{}
	if storeExists {
		if !secretExisted {
			return nil, false, nil, errors.New("credential store exists but its secret is missing; refusing to overwrite")
		}
		data, err = openCredentialStore(secret, blob)
		if err != nil {
			return nil, false, nil, err
		}
	}
	return secret, secretExisted, data, nil
}

// saveCredentialStore persists the secret (when newly generated) and the
// encrypted store back into the container.
func saveCredentialStore(container string, secret []byte, secretExisted bool, data map[string]string) error {
	secretPath, storePath := credentialPaths()
	blob, err := sealCredentialStore(secret, data)
	if err != nil {
		return err
	}
	if !secretExisted {
		if err := writeContainerFile(container, secretPath, secret); err != nil {
			return err
		}
	}
	return writeContainerFile(container, storePath, blob)
}

// SetAPIKey stores the provider API key in the agent's zero credential store
// and marks the provider profile apiKeyStored so zero's resolver loads it. It
// is the web UI's equivalent of zero's TUI "paste an API key" step.
func SetAPIKey(provider, key string, agent ...string) error {
	container := ContainerName(agent...)
	provider = strings.ToLower(strings.TrimSpace(provider))
	key = strings.TrimSpace(key)
	if provider == "" {
		return fmt.Errorf("provider is required")
	}
	if key == "" {
		return fmt.Errorf("API key is required")
	}

	// Zero owns the catalog, so a missing profile is scaffolded through zero
	// itself. An existing profile is left alone: re-running `zero providers
	// add` would overwrite base URL / model customizations.
	exists, err := providerConfigured(container, provider)
	if err != nil {
		return err
	}
	if !exists {
		if _, err := docker("exec", container, "zero", "providers", "add", provider); err != nil {
			return fmt.Errorf("adding provider %q: %w", provider, err)
		}
	}

	secret, secretExisted, data, err := loadCredentialStore(container)
	if err != nil {
		return err
	}
	data[provider] = key
	if err := saveCredentialStore(container, secret, secretExisted, data); err != nil {
		return err
	}
	return markProviderKeyStored(container, provider, true)
}

// DeleteAPIKey removes a provider's stored key and clears its apiKeyStored
// marker so the resolver stops consulting the store for it.
func DeleteAPIKey(provider string, agent ...string) error {
	container := ContainerName(agent...)
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" {
		return fmt.Errorf("provider is required")
	}

	secret, secretExisted, data, err := loadCredentialStore(container)
	if err != nil {
		return err
	}
	delete(data, provider)
	if err := saveCredentialStore(container, secret, secretExisted, data); err != nil {
		return err
	}

	configured, err := providerConfigured(container, provider)
	if err != nil {
		return err
	}
	if configured {
		return markProviderKeyStored(container, provider, false)
	}
	return nil
}

// StoredAPIKeyProviders lists the provider names that have a key in the
// agent's zero credential store, sorted.
func StoredAPIKeyProviders(agent ...string) ([]string, error) {
	container := ContainerName(agent...)
	_, _, data, err := loadCredentialStore(container)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(data))
	for name, key := range data {
		if strings.TrimSpace(key) != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// profileName returns the name field of a provider profile document.
func profileName(p map[string]any) string {
	if v, ok := p["name"].(string); ok {
		return v
	}
	return ""
}

// providerInConfig reports whether a provider profile named provider
// (case-insensitive) exists in a zero config.json document.
func providerInConfig(cfg []byte, provider string) bool {
	var root struct {
		Providers []map[string]any `json:"providers"`
	}
	if json.Unmarshal(cfg, &root) != nil {
		return false
	}
	for _, p := range root.Providers {
		if strings.EqualFold(strings.TrimSpace(profileName(p)), provider) {
			return true
		}
	}
	return false
}

// providerConfigured reports whether the named provider profile already exists
// in the agent's zero config.
func providerConfigured(container, provider string) (bool, error) {
	cfg, exists, err := readContainerFile(container, zeroConfigDir+"/config.json")
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	return providerInConfig(cfg, provider), nil
}

// setProviderKeyStored rewrites a zero config.json document so the named
// provider's profile carries the apiKeyStored marker (or drops it when stored
// is false) and any inline key or env var is cleared, so the stored key is the
// sole runtime credential. Unknown fields survive the round trip.
func setProviderKeyStored(cfg []byte, provider string, stored bool) ([]byte, error) {
	if len(cfg) == 0 {
		return nil, fmt.Errorf("zero config is empty")
	}
	// UseNumber preserves any numeric fields verbatim instead of round-tripping
	// them through float64, so rewriting one provider's marker cannot corrupt
	// unrelated numbers elsewhere in the config.
	var root map[string]any
	dec := json.NewDecoder(bytes.NewReader(cfg))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("invalid zero config: %w", err)
	}
	providers, _ := root["providers"].([]any)
	found := false
	for i, raw := range providers {
		p, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(profileName(p)), provider) {
			continue
		}
		p["apiKeyStored"] = stored
		delete(p, "apiKey")
		delete(p, "apiKeyEnv")
		providers[i] = p
		found = true
	}
	if !found {
		return nil, fmt.Errorf("provider %q not found in zero config", provider)
	}
	root["providers"] = providers
	return json.MarshalIndent(root, "", "  ")
}

// markProviderKeyStored reads the agent's zero config, flips the apiKeyStored
// marker on the named provider, and writes it back.
func markProviderKeyStored(container, provider string, stored bool) error {
	cfgPath := zeroConfigDir + "/config.json"
	cfg, exists, err := readContainerFile(container, cfgPath)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("zero config not found at %s", cfgPath)
	}
	out, err := setProviderKeyStored(cfg, provider, stored)
	if err != nil {
		return err
	}
	return writeContainerFile(container, cfgPath, out)
}
