package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"zeroclaw/internal/catalog"
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

		items, err := catalog.FetchLive(ctx, provider)
		return modelsFetchedMsg{
			provider: provider,
			items:    fromCatalogItems(items),
			err:      err,
		}
	}
}

func fromCatalogItems(items []catalog.Item) []pickerItem {
	result := make([]pickerItem, len(items))
	for i, it := range items {
		result[i] = pickerItem{Group: it.Group, Label: it.Label, Value: it.Value, Meta: it.Meta, Provider: it.Provider}
	}
	return result
}
