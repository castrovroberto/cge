package chat

import (
	"context"

	"github.com/castrovroberto/CGE/internal/config"
)

// ChatModelOption is a functional option for configuring ChatModel
type ChatModelOption func(*Model)

// WithTheme sets the theme for the chat model
func WithTheme(theme *Theme) ChatModelOption {
	return func(m *Model) {
		m.theme = theme
	}
}

// WithMessageProvider sets the message provider for the chat model
func WithMessageProvider(provider MessageProvider) ChatModelOption {
	return func(m *Model) {
		m.messageProvider = provider
	}
}

// WithDelayProvider sets the delay provider for the chat model
func WithDelayProvider(provider DelayProvider) ChatModelOption {
	return func(m *Model) {
		m.delayProvider = provider
	}
}

// WithHistoryService sets the history service for the chat model
func WithHistoryService(service HistoryService) ChatModelOption {
	return func(m *Model) {
		m.historyService = service
	}
}

// WithParentContext sets the parent context for the chat model
func WithParentContext(ctx context.Context) ChatModelOption {
	return func(m *Model) {
		m.parentCtx = ctx
	}
}

// WithInitialConfig sets the app configuration for the chat model
func WithInitialConfig(cfg *config.AppConfig) ChatModelOption {
	return func(m *Model) {
		m.cfg = cfg
	}
}

// WithAvailableCommands sets the available slash commands for the chat model
func WithAvailableCommands(commands []string) ChatModelOption {
	return func(m *Model) {
		m.availableCommands = commands
	}
}

// WithHeader sets the header model for the chat model
func WithHeader(header *HeaderModel) ChatModelOption {
	return func(m *Model) {
		m.header = header
	}
}
