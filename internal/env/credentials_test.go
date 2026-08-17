package env

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCredentialStoreRoundTrip(t *testing.T) {
	secret := bytes.Repeat([]byte{0x42}, credentialSecretSize)

	blob, err := sealCredentialStore(secret, map[string]string{"openai": "sk-abc", "anthropic": "sk-def"})
	if err != nil {
		t.Fatalf("sealCredentialStore: %v", err)
	}
	data, err := openCredentialStore(secret, blob)
	if err != nil {
		t.Fatalf("openCredentialStore: %v", err)
	}
	if data["openai"] != "sk-abc" || data["anthropic"] != "sk-def" {
		t.Fatalf("round trip mismatch: %v", data)
	}

	// A wrong secret must fail closed, matching zero's securefile.
	bad := append([]byte(nil), secret...)
	bad[0] ^= 0xff
	if _, err := openCredentialStore(bad, blob); err == nil {
		t.Fatal("expected an error decrypting with the wrong secret")
	}

	// A truncated blob must fail, not panic.
	if _, err := openCredentialStore(secret, []byte("short")); err == nil {
		t.Fatal("expected an error for a short blob")
	}
}

func TestSetProviderKeyStoredPreservesUnknownFields(t *testing.T) {
	cfg := []byte(`{
		"activeProvider": "openai",
		"providers": [
			{
				"name": "OpenAI",
				"catalogID": "openai",
				"baseURL": "https://api.openai.com/v1",
				"apiKey": "sk-old",
				"apiKeyEnv": "OPENAI_API_KEY",
				"custom": 42
			}
		],
		"sandbox": {"network": "allow"}
	}`)

	out, err := setProviderKeyStored(cfg, "openai", true)
	if err != nil {
		t.Fatalf("setProviderKeyStored: %v", err)
	}

	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	if root["sandbox"] == nil {
		t.Error("unknown top-level field sandbox was dropped")
	}
	if root["activeProvider"] != "openai" {
		t.Errorf("activeProvider = %v, want openai", root["activeProvider"])
	}
	providers, ok := root["providers"].([]any)
	if !ok || len(providers) != 1 {
		t.Fatalf("providers = %v, want one entry", root["providers"])
	}
	p, ok := providers[0].(map[string]any)
	if !ok {
		t.Fatalf("provider entry is not an object: %v", providers[0])
	}
	if p["apiKeyStored"] != true {
		t.Errorf("apiKeyStored = %v, want true", p["apiKeyStored"])
	}
	if _, exists := p["apiKey"]; exists {
		t.Error("inline apiKey was not cleared")
	}
	if _, exists := p["apiKeyEnv"]; exists {
		t.Error("apiKeyEnv was not cleared")
	}
	if n, ok := p["custom"].(float64); !ok || n != 42 {
		t.Errorf("custom = %v (%T), want float64 42", p["custom"], p["custom"])
	}
}

func TestSetProviderKeyStoredClearsMarker(t *testing.T) {
	cfg := []byte(`{"providers":[{"name":"openai","apiKeyStored":true}]}`)
	out, err := setProviderKeyStored(cfg, "openai", false)
	if err != nil {
		t.Fatalf("setProviderKeyStored: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(out, &root); err != nil {
		t.Fatalf("output not valid JSON: %v", err)
	}
	providers := root["providers"].([]any)
	p := providers[0].(map[string]any)
	if p["apiKeyStored"] != false {
		t.Errorf("apiKeyStored = %v, want false", p["apiKeyStored"])
	}
}

func TestSetProviderKeyStoredNotFound(t *testing.T) {
	if _, err := setProviderKeyStored([]byte(`{"providers":[]}`), "openai", true); err == nil {
		t.Fatal("expected an error when the provider is absent")
	}
}

func TestProviderInConfig(t *testing.T) {
	cfg := []byte(`{"providers":[{"name":"OpenAI"},{"name":"anthropic"}]}`)
	if !providerInConfig(cfg, "openai") {
		t.Error("providerInConfig(openai) = false, want true (case-insensitive)")
	}
	if !providerInConfig(cfg, "anthropic") {
		t.Error("providerInConfig(anthropic) = false, want true")
	}
	if providerInConfig(cfg, "nope") {
		t.Error("providerInConfig(nope) = true, want false")
	}
	if providerInConfig([]byte(`{}`), "openai") {
		t.Error("providerInConfig with no providers = true, want false")
	}
}
