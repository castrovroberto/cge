package chat

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestHeaderModel(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *HeaderModel
		action   func(*HeaderModel) interface{}
		expected interface{}
	}{
		{
			name: "new_header_has_correct_values",
			setup: func() *HeaderModel {
				theme := NewDefaultTheme()
				return NewHeaderModel(theme, "TestProvider", "test-model", "session123", "Connected")
			},
			action: func(model *HeaderModel) interface{} {
				return []string{model.GetProvider(), model.GetModelName(), model.GetSessionID(), model.GetStatus()}
			},
			expected: []string{"TestProvider", "test-model", "session123", "Connected"},
		},
		{
			name: "new_header_has_uuid_and_system_info",
			setup: func() *HeaderModel {
				theme := NewDefaultTheme()
				return NewHeaderModel(theme, "TestProvider", "test-model", "session123", "Connected")
			},
			action: func(model *HeaderModel) interface{} {
				return map[string]interface{}{
					"hasUUID":           len(model.GetSessionUUID()) > 0,
					"hasWorkingDir":     len(model.GetWorkingDirectory()) > 0,
					"sessionTimeExists": !model.GetSessionTime().IsZero(),
				}
			},
			expected: map[string]interface{}{
				"hasUUID":           true,
				"hasWorkingDir":     true,
				"sessionTimeExists": true,
			},
		},
		{
			name: "set_provider_updates_value",
			setup: func() *HeaderModel {
				theme := NewDefaultTheme()
				return NewHeaderModel(theme, "OldProvider", "model", "session", "status")
			},
			action: func(model *HeaderModel) interface{} {
				model.SetProvider("NewProvider")
				return model.GetProvider()
			},
			expected: "NewProvider",
		},
		{
			name: "set_model_name_updates_value",
			setup: func() *HeaderModel {
				theme := NewDefaultTheme()
				return NewHeaderModel(theme, "provider", "old-model", "session", "status")
			},
			action: func(model *HeaderModel) interface{} {
				model.SetModelName("new-model")
				return model.GetModelName()
			},
			expected: "new-model",
		},
		{
			name: "set_session_id_updates_value",
			setup: func() *HeaderModel {
				theme := NewDefaultTheme()
				return NewHeaderModel(theme, "provider", "model", "old-session", "status")
			},
			action: func(model *HeaderModel) interface{} {
				model.SetSessionID("new-session")
				return model.GetSessionID()
			},
			expected: "new-session",
		},
		{
			name: "set_status_updates_value",
			setup: func() *HeaderModel {
				theme := NewDefaultTheme()
				return NewHeaderModel(theme, "provider", "model", "session", "Disconnected")
			},
			action: func(model *HeaderModel) interface{} {
				model.SetStatus("Connected")
				return model.GetStatus()
			},
			expected: "Connected",
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

func TestHeaderModelView(t *testing.T) {
	theme := NewDefaultTheme()
	header := NewHeaderModel(theme, "Ollama", "llama2", "20240101123000", "Connected")

	// Test narrow terminal (compact mode)
	narrowResize := tea.WindowSizeMsg{Width: 70, Height: 50}
	header, _ = header.Update(narrowResize)
	compactView := header.View()

	// Verify the compact view contains key information
	assert.Contains(t, compactView, "CGE", "Should show application name")
	assert.Contains(t, compactView, "Ollama", "Should show provider")
	assert.Contains(t, compactView, "llama2", "Should show model name")
	assert.Contains(t, compactView, "Connected", "Should show status")
	assert.NotEmpty(t, compactView, "Compact view should not be empty")

	// Test wide terminal (bordered mode)
	wideResize := tea.WindowSizeMsg{Width: 100, Height: 50}
	header, _ = header.Update(wideResize)
	borderedView := header.View()

	// Verify the bordered view contains key information
	assert.Contains(t, borderedView, "CGE Chat", "Should show application name in bordered view")
	assert.Contains(t, borderedView, "Ollama", "Should show provider in bordered view")
	assert.Contains(t, borderedView, "llama2", "Should show model name in bordered view")
	assert.Contains(t, borderedView, "connected", "Should show status in bordered view (lowercase)")
	assert.Contains(t, borderedView, "localhost session:", "Should show session info")
	assert.Contains(t, borderedView, "↳ workdir:", "Should show working directory with arrow")
	assert.Contains(t, borderedView, "↳ model:", "Should show model with arrow")
	assert.Contains(t, borderedView, "↳ provider:", "Should show provider with arrow")
	assert.Contains(t, borderedView, "↳ status:", "Should show status with arrow")

	// Check for border characters (the exact characters may vary with lipgloss)
	assert.True(t, strings.Contains(borderedView, "╭") || strings.Contains(borderedView, "┌"), "Should contain top border")
	assert.True(t, strings.Contains(borderedView, "╰") || strings.Contains(borderedView, "└"), "Should contain bottom border")

	assert.NotEmpty(t, borderedView, "Bordered view should not be empty")
}

func TestHeaderModelDynamicHeight(t *testing.T) {
	theme := NewDefaultTheme()
	header := NewHeaderModel(theme, "Ollama", "model", "session", "Connected")

	// Test compact mode (narrow terminal)
	narrowResize := tea.WindowSizeMsg{Width: 70, Height: 50}
	header, _ = header.Update(narrowResize)

	compactHeight := header.GetHeight()
	assert.Equal(t, theme.HeaderHeight, compactHeight, "Should use theme default height for narrow terminals")

	// Test bordered mode (wide terminal)
	wideResize := tea.WindowSizeMsg{Width: 100, Height: 50}
	header, _ = header.Update(wideResize)

	borderedHeight := header.GetHeight()
	// App box: 3 lines; separator: 1; session box: sessionLineCount() content + 2 border lines
	assert.Greater(t, borderedHeight, 7, "Should use more than 7 lines for bordered layout with full session content")
}

func TestHeaderModelUpdate(t *testing.T) {
	theme := NewDefaultTheme()
	header := NewHeaderModel(theme, "Ollama", "model", "session", "Connected")

	// Test window resize updates width and layout mode
	resizeMsg := tea.WindowSizeMsg{Width: 100, Height: 50}
	updatedHeader, cmd := header.Update(resizeMsg)

	// Should update without error
	assert.NotNil(t, updatedHeader, "Updated header should not be nil")
	assert.Nil(t, cmd, "Window resize should not produce commands")
	assert.Equal(t, 100, updatedHeader.width, "Should update width")
	assert.True(t, updatedHeader.multiLine, "Should enable multiline for wide terminals")

	// Test narrow resize
	narrowResize := tea.WindowSizeMsg{Width: 70, Height: 50}
	updatedHeader, cmd = updatedHeader.Update(narrowResize)
	assert.False(t, updatedHeader.multiLine, "Should disable multiline for narrow terminals")

	// Test other message types (should be ignored)
	keyMsg := tea.KeyMsg{Type: tea.KeyEnter}
	updatedHeader, cmd = header.Update(keyMsg)
	assert.NotNil(t, updatedHeader, "Updated header should not be nil")
	assert.Nil(t, cmd, "Key messages should not produce commands")
}

func TestHeaderModelGetters(t *testing.T) {
	theme := NewDefaultTheme()
	header := NewHeaderModel(theme, "TestProvider", "test-model", "test-session", "TestStatus")

	// Test all getters
	assert.Equal(t, "TestProvider", header.GetProvider())
	assert.Equal(t, "test-model", header.GetModelName())
	assert.Equal(t, "test-session", header.GetSessionID())
	assert.Equal(t, "TestStatus", header.GetStatus())

	// Test new getters
	assert.NotEmpty(t, header.GetSessionUUID(), "Should have a session UUID")
	assert.NotEmpty(t, header.GetWorkingDirectory(), "Should have a working directory")
	assert.False(t, header.GetSessionTime().IsZero(), "Should have a session time")
	assert.Equal(t, "v1.0.0", header.GetVersion(), "Should have default version")
}

func TestHeaderModelSetters(t *testing.T) {
	theme := NewDefaultTheme()
	header := NewHeaderModel(theme, "Initial", "initial", "initial", "Initial")

	// Test all setters
	header.SetProvider("Updated Provider")
	header.SetModelName("updated-model")
	header.SetSessionID("updated-session")
	header.SetStatus("Updated Status")
	header.SetVersion("v2.0.0")

	// Verify updates
	assert.Equal(t, "Updated Provider", header.GetProvider())
	assert.Equal(t, "updated-model", header.GetModelName())
	assert.Equal(t, "updated-session", header.GetSessionID())
	assert.Equal(t, "Updated Status", header.GetStatus())
	assert.Equal(t, "v2.0.0", header.GetVersion())
}

func TestHeaderModelGitInfo(t *testing.T) {
	theme := NewDefaultTheme()
	header := NewHeaderModel(theme, "Ollama", "model", "session", "Connected")

	// Test git info methods exist and don't panic
	assert.NotNil(t, header.GetGitBranch(), "Git branch should not be nil")
	assert.NotPanics(t, func() { header.IsGitRepo() }, "IsGitRepo should not panic")
	assert.NotPanics(t, func() { header.RefreshGitInfo() }, "RefreshGitInfo should not panic")
}
