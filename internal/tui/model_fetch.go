package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type modelsFetchedMsg struct {
	provider string
	items    []pickerItem
	err      error
}

func fetchModelsCmd(provider string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		items, err := fetchLiveModels(ctx, provider)
		return modelsFetchedMsg{
			provider: provider,
			items:    items,
			err:      err,
		}
	}
}

func fetchLiveModels(ctx context.Context, provider string) ([]pickerItem, error) {
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
	req.Header.Set("User-Agent", "zeroclaw-cli")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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

func parseLiveModelsResponse(body []byte, provider string) ([]pickerItem, error) {
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

	var result []pickerItem
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

		meta := formatModelMeta(ctx, isFree)

		result = append(result, pickerItem{
			Group:    groupName,
			Label:    label,
			Value:    id,
			Meta:     meta,
			Provider: provider,
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
