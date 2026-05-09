package chat

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestMessageListModel(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *MessageListModel
		action   func(*MessageListModel) interface{}
		expected interface{}
	}{
		{
			name: "new_message_list_has_correct_dimensions",
			setup: func() *MessageListModel {
				theme := NewDefaultTheme()
				return NewMessageListModel(theme, 80, 20)
			},
			action: func(model *MessageListModel) interface{} {
				return []int{model.width, model.height}
			},
			expected: []int{80, 20},
		},
		{
			name: "add_message_increases_count",
			setup: func() *MessageListModel {
				theme := NewDefaultTheme()
				return NewMessageListModel(theme, 80, 20)
			},
			action: func(model *MessageListModel) interface{} {
				initialCount := len(model.GetMessages())
				model.AddMessage(chatMessage{
					text:      "Test message",
					sender:    "User",
					timestamp: time.Now(),
				})
				return len(model.GetMessages()) - initialCount
			},
			expected: 1,
		},
		{
			name: "placeholder_replacement_works",
			setup: func() *MessageListModel {
				theme := NewDefaultTheme()
				model := NewMessageListModel(theme, 80, 20)
				// Add a placeholder
				model.AddMessage(chatMessage{
					text:        "...",
					sender:      "Assistant",
					timestamp:   time.Now(),
					placeholder: true,
				})
				return model
			},
			action: func(model *MessageListModel) interface{} {
				model.ReplacePlaceholder(chatMessage{
					text:      "Real response",
					sender:    "Assistant",
					timestamp: time.Now(),
				})
				messages := model.GetMessages()
				lastMessage := messages[len(messages)-1]
				return []interface{}{lastMessage.text, lastMessage.placeholder}
			},
			expected: []interface{}{"Real response", false},
		},
		{
			name: "set_height_updates_viewport",
			setup: func() *MessageListModel {
				theme := NewDefaultTheme()
				return NewMessageListModel(theme, 80, 20)
			},
			action: func(model *MessageListModel) interface{} {
				model.SetHeight(30)
				return model.GetHeight()
			},
			expected: 30,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := tt.setup()
			result := tt.action(model)
			assert.Equal(t, tt.expected, result)
		})
	}
}


func TestMessageListMessageTypes(t *testing.T) {
	tests := []struct {
		name        string
		message     chatMessage
		expectation func(*testing.T, *MessageListModel)
	}{
		{
			name: "regular_message",
			message: chatMessage{
				text:      "Hello world",
				sender:    "User",
				timestamp: time.Now(),
			},
			expectation: func(t *testing.T, model *MessageListModel) {
				messages := model.GetMessages()
				assert.Len(t, messages, 1)
				assert.Equal(t, "Hello world", messages[0].text)
				assert.Equal(t, "User", messages[0].sender)
			},
		},
		{
			name: "markdown_message",
			message: chatMessage{
				text:       "# Header\n\nSome **bold** text",
				sender:     "Assistant",
				timestamp:  time.Now(),
				isMarkdown: true,
			},
			expectation: func(t *testing.T, model *MessageListModel) {
				messages := model.GetMessages()
				assert.Len(t, messages, 1)
				assert.True(t, messages[0].isMarkdown)
			},
		},
		{
			name: "tool_call_message",
			message: chatMessage{
				text:       "Executing search...",
				sender:     "System",
				timestamp:  time.Now(),
				isToolCall: true,
				toolName:   "search",
				toolCallID: "call-123",
				toolParams: map[string]interface{}{"query": "test"},
			},
			expectation: func(t *testing.T, model *MessageListModel) {
				messages := model.GetMessages()
				assert.Len(t, messages, 1)
				assert.True(t, messages[0].isToolCall)
				assert.Equal(t, "search", messages[0].toolName)
				assert.Equal(t, "call-123", messages[0].toolCallID)
			},
		},
		{
			name: "tool_result_message",
			message: chatMessage{
				text:         "Search completed successfully",
				sender:       "System",
				timestamp:    time.Now(),
				isToolResult: true,
				toolName:     "search",
				toolSuccess:  true,
				toolDuration: time.Second * 2,
			},
			expectation: func(t *testing.T, model *MessageListModel) {
				messages := model.GetMessages()
				assert.Len(t, messages, 1)
				assert.True(t, messages[0].isToolResult)
				assert.True(t, messages[0].toolSuccess)
				assert.Equal(t, time.Second*2, messages[0].toolDuration)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			theme := NewDefaultTheme()
			model := NewMessageListModel(theme, 80, 20)
			model.AddMessage(tt.message)
			tt.expectation(t, model)
		})
	}
}

func TestMessageListWindowResize(t *testing.T) {
	theme := NewDefaultTheme()
	model := NewMessageListModel(theme, 80, 20)

	// Add some test messages
	model.AddMessage(chatMessage{
		text:      "Test message 1",
		sender:    "User",
		timestamp: time.Now(),
	})

	// Simulate window resize: parent calls SetHeight after layout calc, Update handles width
	resizeMsg := tea.WindowSizeMsg{Width: 100, Height: 30}
	_, cmd := model.Update(resizeMsg)
	model.SetHeight(30)

	wFrame := model.viewport.Style.GetHorizontalFrameSize()
	// Verify dimensions updated (width stored as inner width minus frame)
	assert.Equal(t, 100-wFrame, model.width)
	assert.Equal(t, 30, model.height)
	assert.Nil(t, cmd, "Resize should not produce commands")
}

func TestMessageListLoadHistory(t *testing.T) {
	theme := NewDefaultTheme()
	model := NewMessageListModel(theme, 80, 20)

	// Create test messages
	testMessages := []chatMessage{
		{
			text:      "Message 1",
			sender:    "User",
			timestamp: time.Now(),
		},
		{
			text:      "Message 2",
			sender:    "Assistant",
			timestamp: time.Now(),
		},
	}

	// Load history
	model.LoadHistory(testMessages)

	// Verify messages loaded
	messages := model.GetMessages()
	assert.Len(t, messages, 2)
	assert.Equal(t, "Message 1", messages[0].text)
	assert.Equal(t, "Message 2", messages[1].text)
}

func TestMessageListCodeBlockProcessing(t *testing.T) {
	theme := NewDefaultTheme()
	model := NewMessageListModel(theme, 80, 20)

	// Add message with code block
	codeMessage := chatMessage{
		text:       "Here's some code:\n```go\nfunc main() {\n    fmt.Println(\"Hello\")\n}\n```",
		sender:     "Assistant",
		timestamp:  time.Now(),
		isMarkdown: true,
	}

	model.AddMessage(codeMessage)

	// Verify message added
	messages := model.GetMessages()
	assert.Len(t, messages, 1)
	assert.True(t, messages[0].isMarkdown)

	// The processCodeBlocks function should have been called during AddMessage
	// We can't easily test the exact output without complex setup, but we can verify
	// the message was processed without error
	view := model.View()
	assert.NotEmpty(t, view, "View should render without error")
}
