package chat

import (
	"context"
	"time"
)

// MessageType represents the type of a chat message
type MessageType int

const (
	UserMessage MessageType = iota
	AssistantMessage
	ToolCallMessage   // For displaying tool call attempts
	ToolResultMessage // For displaying tool results
	ErrorMessage
	SystemMessage
	TurnComplete // Signals the end of a presenter turn; no display content
)

// ChatMessage represents a generic message in the chat system
type ChatMessage struct {
	ID        string                 `json:"id"`        // Unique ID for the message
	Type      MessageType            `json:"type"`      // Type of message (user, assistant, error, tool, etc.)
	Sender    string                 `json:"sender"`    // "User", "Assistant", "System", ToolName
	Text      string                 `json:"text"`      // Main content of the message
	Timestamp time.Time              `json:"timestamp"` // When the message was created
	Metadata  map[string]interface{} `json:"metadata"`  // For additional data like tool call details, error codes, etc.
}

// TurnCompleteMessage is sent by the MessageProvider after every turn (success or error)
// so the TUI can reliably clear loading state.
type TurnCompleteMessage struct{}

// ProviderClosedMessage is delivered by listenForMessages when the provider's channel
// is closed intentionally (e.g. on quit). It is distinct from an error.
type ProviderClosedMessage struct{}

// MessageProvider handles sending prompts and receiving chat messages.
type MessageProvider interface {
	// Send initiates the processing of a user's prompt.
	// It should not block and should return an error immediately if submission fails.
	Send(ctx context.Context, prompt string) error

	// Messages returns a channel that streams ChatMessage instances.
	// The TUI will listen to this channel for updates.
	Messages() <-chan ChatMessage

	// Close allows for cleanup of the message provider resources.
	Close() error
}
