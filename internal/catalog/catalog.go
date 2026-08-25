// Package catalog holds the curated model/provider lists and the live model
// fetch behind the web UI's /model and /provider pickers, served by the
// /providers and /models routes in internal/daemon.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Item is one selectable entry in a model or provider picker.
type Item struct {
	Group    string `json:"group"`
	Label    string `json:"label"`
	Value    string `json:"value"`
	Meta     string `json:"meta"`
	Provider string `json:"provider"`
	// ContextWindow is the model's context size in tokens, 0 when unknown.
	// Kept as a number rather than only inside Meta so callers can use it
	// without parsing display text back apart.
	ContextWindow int `json:"contextWindow,omitempty"`
	// KeyAuth marks providers that authenticate with a plain API key, which
	// the web UI's /auth flow can store. Local and custom-endpoint providers
	// are excluded: they either need no key or need more than a key.
	KeyAuth bool `json:"keyAuth,omitempty"`
}

var providers = []Item{
	{Group: "Providers", Label: "GitLawb OpenGateway", Value: "gitlawb-opengateway", Meta: "smart-routing gateway", KeyAuth: true},
	{Group: "Providers", Label: "OpenRouter", Value: "openrouter", Meta: "multi-provider gateway", KeyAuth: true},
	{Group: "Providers", Label: "Anthropic", Value: "anthropic", Meta: "Claude models", KeyAuth: true},
	{Group: "Providers", Label: "OpenAI", Value: "openai", Meta: "GPT & o-series models", KeyAuth: true},
	{Group: "Providers", Label: "DeepSeek", Value: "deepseek", Meta: "V4 Pro & Flash models", KeyAuth: true},
	{Group: "Providers", Label: "Google AI", Value: "google", Meta: "Gemini models", KeyAuth: true},
	{Group: "Providers", Label: "Ollama (Local)", Value: "ollama", Meta: "Local models"},
	{Group: "Providers", Label: "AIMLAPI", Value: "aimlapi", Meta: "AI/ML API gateway", KeyAuth: true},
	{Group: "Providers", Label: "Custom (OpenAI-compatible)", Value: "custom-openai-compatible", Meta: "Custom endpoint"},
}

// Providers lists the pickable model providers.
func Providers() []Item {
	return providers
}

// KeyedAuth reports whether provider accepts a plain API key via the web UI's
// /auth flow. The daemon uses this to reject key capture for providers that
// need no key (local) or need more than a key (custom endpoint + base URL).
func KeyedAuth(provider string) bool {
	// Optimization: O(1) map lookup instead of O(N) iteration
	return keyedAuthMap[strings.ToLower(provider)]
}

// LiveFetchSupported reports whether provider has a live model catalog
// endpoint (as opposed to only the curated static list below).
func LiveFetchSupported(provider string) bool {
	// Optimization: strings.EqualFold is faster and allocates less than map lookup + ToLower
	return strings.EqualFold(provider, "gitlawb-opengateway") || strings.EqualFold(provider, "openrouter")
}

// StaticModels returns the curated model list, optionally filtered to one
// provider. An empty provider returns everything.
var staticModels = []Item{
	{Group: "GitLawb OpenGateway", Label: "NVIDIA Nemotron 3 Ultra 550B (Free)", Value: "nvidia/nemotron-3-ultra-550b-a55b:free", Meta: "128K ctx · free", Provider: "gitlawb-opengateway", ContextWindow: 128000},
	{Group: "GitLawb OpenGateway", Label: "Mimo v2.5 Pro", Value: "mimo-v2.5-pro", Meta: "128K ctx · smart-route", Provider: "gitlawb-opengateway", ContextWindow: 128000},
	{Group: "GitLawb OpenGateway", Label: "Qwen 3 Coder 480B", Value: "qwen3-coder:480b", Meta: "128K ctx · code", Provider: "gitlawb-opengateway", ContextWindow: 128000},

	{Group: "OpenRouter", Label: "DeepSeek V4 Pro", Value: "deepseek/deepseek-v4-pro", Meta: "1M ctx · tools", Provider: "openrouter", ContextWindow: 1000000},
	{Group: "OpenRouter", Label: "DeepSeek V4 Flash", Value: "deepseek/deepseek-v4-flash-0731", Meta: "1M ctx · fast · tools", Provider: "openrouter", ContextWindow: 1000000},
	{Group: "OpenRouter", Label: "Claude Sonnet 4.5", Value: "anthropic/claude-sonnet-4.5", Meta: "200K ctx · tools · vision", Provider: "openrouter", ContextWindow: 200000},
	{Group: "OpenRouter", Label: "Claude Haiku 4.5", Value: "anthropic/claude-haiku-4.5", Meta: "200K ctx · fast · tools", Provider: "openrouter", ContextWindow: 200000},
	{Group: "OpenRouter", Label: "Gemini 2.5 Flash", Value: "google/gemini-2.5-flash", Meta: "1M ctx · fast · vision", Provider: "openrouter", ContextWindow: 1000000},
	{Group: "OpenRouter", Label: "Gemini 2.5 Pro", Value: "google/gemini-2.5-pro", Meta: "1M ctx · reasoning · vision", Provider: "openrouter", ContextWindow: 1000000},
	{Group: "OpenRouter", Label: "GPT-4.1", Value: "openai/gpt-4.1", Meta: "1M ctx · tools · vision", Provider: "openrouter", ContextWindow: 1000000},
	{Group: "OpenRouter", Label: "GPT-4.1 Mini", Value: "openai/gpt-4.1-mini", Meta: "1M ctx · fast · tools", Provider: "openrouter", ContextWindow: 1000000},
	{Group: "OpenRouter", Label: "o3-mini", Value: "openai/o3-mini", Meta: "200K ctx · reasoning", Provider: "openrouter", ContextWindow: 200000},
	{Group: "OpenRouter", Label: "Llama 3.3 70B Instruct", Value: "meta-llama/llama-3.3-70b-instruct", Meta: "128K ctx · tools", Provider: "openrouter", ContextWindow: 128000},

	{Group: "Anthropic", Label: "Claude Sonnet 4.5", Value: "claude-sonnet-4.5", Meta: "200K ctx · tools · vision", Provider: "anthropic", ContextWindow: 200000},
	{Group: "Anthropic", Label: "Claude Haiku 4.5", Value: "claude-haiku-4.5", Meta: "200K ctx · fast · tools", Provider: "anthropic", ContextWindow: 200000},
	{Group: "Anthropic", Label: "Claude Opus 4.1", Value: "claude-opus-4.1", Meta: "200K ctx · reasoning", Provider: "anthropic", ContextWindow: 200000},

	{Group: "OpenAI", Label: "GPT-4.1", Value: "gpt-4.1", Meta: "1M ctx · tools · vision", Provider: "openai", ContextWindow: 1000000},
	{Group: "OpenAI", Label: "GPT-4.1 Mini", Value: "gpt-4.1-mini", Meta: "1M ctx · fast · tools", Provider: "openai", ContextWindow: 1000000},
	{Group: "OpenAI", Label: "GPT-4.1 Nano", Value: "gpt-4.1-nano", Meta: "1M ctx · fast · lightweight", Provider: "openai", ContextWindow: 1000000},
	{Group: "OpenAI", Label: "o3-mini", Value: "o3-mini", Meta: "200K ctx · reasoning", Provider: "openai", ContextWindow: 200000},
	{Group: "OpenAI", Label: "o1", Value: "o1", Meta: "200K ctx · reasoning", Provider: "openai", ContextWindow: 200000},
	{Group: "OpenAI", Label: "GPT-4o", Value: "gpt-4o", Meta: "128K ctx · tools · vision", Provider: "openai", ContextWindow: 128000},
	{Group: "OpenAI", Label: "GPT-4o Mini", Value: "gpt-4o-mini", Meta: "128K ctx · fast · tools", Provider: "openai", ContextWindow: 128000},

	{Group: "DeepSeek", Label: "DeepSeek V4 Pro", Value: "deepseek-chat", Meta: "1M ctx · tools", Provider: "deepseek", ContextWindow: 1000000},
	{Group: "DeepSeek", Label: "DeepSeek V4 Flash", Value: "deepseek-reasoner", Meta: "1M ctx · fast · reasoning", Provider: "deepseek", ContextWindow: 1000000},

	{Group: "Google AI", Label: "Gemini 2.5 Flash", Value: "gemini-2.5-flash", Meta: "1M ctx · fast · vision", Provider: "google", ContextWindow: 1000000},
	{Group: "Google AI", Label: "Gemini 2.5 Pro", Value: "gemini-2.5-pro", Meta: "1M ctx · reasoning · vision", Provider: "google", ContextWindow: 1000000},
	{Group: "Google AI", Label: "Gemini 2.5 Flash-Lite", Value: "gemini-2.5-flash-lite", Meta: "1M ctx · fast · lightweight", Provider: "google", ContextWindow: 1000000},

	{Group: "Ollama (Local)", Label: "Llama 3.3 70B", Value: "llama3.3", Meta: "local", Provider: "ollama"},
	{Group: "Ollama (Local)", Label: "Qwen 2.5 Coder 32B", Value: "qwen2.5-coder", Meta: "local · code", Provider: "ollama"},
	{Group: "Ollama (Local)", Label: "DeepSeek R1 Distill 32B", Value: "deepseek-r1", Meta: "local · reasoning", Provider: "ollama"},
}

var (
	modelsByProvider map[string][]Item
	contextWindows   map[string]int
	keyedAuthMap     map[string]bool
)

func init() {
	modelsByProvider = make(map[string][]Item)
	contextWindows = make(map[string]int)

	// Pre-allocate map slice capacities
	counts := make(map[string]int)
	for i := range staticModels {
		counts[strings.ToLower(staticModels[i].Provider)]++
	}

	for k, v := range counts {
		modelsByProvider[k] = make([]Item, 0, v)
	}

	for i := range staticModels {
		item := staticModels[i]
		lowerProvider := strings.ToLower(item.Provider)
		modelsByProvider[lowerProvider] = append(modelsByProvider[lowerProvider], item)

		lowerValue := strings.ToLower(item.Value)
		contextWindows[lowerValue] = item.ContextWindow
	}

	keyedAuthMap = make(map[string]bool, len(providers))
	for i := range providers {
		if providers[i].KeyAuth {
			keyedAuthMap[strings.ToLower(providers[i].Value)] = true
		}
	}

}

func StaticModels(provider string) []Item {
	if provider == "" {
		return staticModels
	}
	// Optimization: O(1) map lookup instead of O(N) iteration
	return modelsByProvider[strings.ToLower(provider)]
}

// ContextWindowFor reports the curated context size for a model id, or 0
// when the catalog does not list it. Used as a fallback when the backend's
// own model registry has no entry for the active model, which happens for
// gateway-routed ids the registry does not enumerate.
func ContextWindowFor(model string) int {
	if model == "" {
		return 0
	}
	// Optimization: O(1) map lookup instead of O(N) iteration
	return contextWindows[strings.ToLower(model)]
}

// FormatContextWindow renders a token count the way the pickers do
// ("128K ctx", "1M ctx"), or "" when unknown.
func FormatContextWindow(ctx int) string { return formatContextWindow(ctx) }

// FetchLive queries provider's live model catalog endpoint. Only
// LiveFetchSupported providers have one.
func FetchLive(ctx context.Context, provider string) ([]Item, error) {
	var endpoint string
	switch provider {
	case "gitlawb-opengateway":
		endpoint = "https://opengateway.gitlawb.com/v1/models"
	case "openrouter":
		endpoint = "https://openrouter.ai/api/v1/models"
	default:
		return nil, fmt.Errorf("unsupported provider for live fetch: %s", provider)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "zeroclaw")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("models endpoint returned status %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}

	return parseLiveModelsResponse(body, provider)
}

type rawModelItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	ContextWindow int    `json:"context_window"`
	ContextLength int    `json:"context_length"`
	Pricing       *struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
}

type rawModelsResponse struct {
	Data   []rawModelItem `json:"data"`
	Models []rawModelItem `json:"models"`
}

func parseLiveModelsResponse(body []byte, provider string) ([]Item, error) {
	var resp rawModelsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	items := resp.Data
	if len(items) == 0 {
		items = resp.Models
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("no models found in catalog response")
	}

	groupName := "OpenRouter"
	if provider == "gitlawb-opengateway" {
		groupName = "GitLawb OpenGateway"
	}

	var result []Item
	for _, m := range items {
		id := strings.TrimSpace(m.ID)
		if id == "" {
			continue
		}

		label := strings.TrimSpace(m.Name)
		if label == "" {
			label = id
		}

		ctx := m.ContextWindow
		if ctx <= 0 {
			ctx = m.ContextLength
		}

		isFree := strings.HasSuffix(strings.ToLower(id), ":free") ||
			(m.Pricing != nil && m.Pricing.Prompt == "0" && m.Pricing.Completion == "0")

		result = append(result, Item{
			Group:         groupName,
			Label:         label,
			Value:         id,
			Meta:          formatModelMeta(ctx, isFree),
			Provider:      provider,
			ContextWindow: ctx,
		})
	}
	return result, nil
}

func formatContextWindow(ctx int) string {
	if ctx <= 0 {
		return ""
	}
	if ctx >= 1_000_000 {
		m := float64(ctx) / 1_000_000.0
		if m == float64(int(m)) {
			return fmt.Sprintf("%dM ctx", int(m))
		}
		return fmt.Sprintf("%.1fM ctx", m)
	}
	k := (ctx + 500) / 1000
	return fmt.Sprintf("%dK ctx", k)
}

func formatModelMeta(ctx int, isFree bool) string {
	ctxStr := formatContextWindow(ctx)
	if isFree {
		if ctxStr != "" {
			return ctxStr + " · free"
		}
		return "free"
	}
	return ctxStr
}
