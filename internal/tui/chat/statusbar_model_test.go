package chat

import (
	"errors"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
)

func TestStatusBarModel(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *StatusBarModel
		action   func(*StatusBarModel) interface{}
		expected interface{}
	}{
		{
			name: "new_status_bar_not_loading",
			setup: func() *StatusBarModel {
				theme := NewDefaultTheme()
				return NewStatusBarModel(theme, time.Now())
			},
			action: func(model *StatusBarModel) interface{} {
				return model.loading
			},
			expected: false,
		},
		{
			name: "set_loading_true_updates_state",
			setup: func() *StatusBarModel {
				theme := NewDefaultTheme()
				return NewStatusBarModel(theme, time.Now())
			},
			action: func(model *StatusBarModel) interface{} {
				model.SetLoading(true)
				return model.loading
			},
			expected: true,
		},
		{
			name: "set_loading_false_updates_state",
			setup: func() *StatusBarModel {
				theme := NewDefaultTheme()
				model := NewStatusBarModel(theme, time.Now())
				model.SetLoading(true)
				return model
			},
			action: func(model *StatusBarModel) interface{} {
				model.SetLoading(false)
				return model.loading
			},
			expected: false,
		},
		{
			name: "set_error_stores_error",
			setup: func() *StatusBarModel {
				theme := NewDefaultTheme()
				return NewStatusBarModel(theme, time.Now())
			},
			action: func(model *StatusBarModel) interface{} {
				testErr := errors.New("test error")
				model.SetError(testErr)
				return model.err != nil
			},
			expected: true,
		},
		{
			name: "clear_error_removes_error",
			setup: func() *StatusBarModel {
				theme := NewDefaultTheme()
				model := NewStatusBarModel(theme, time.Now())
				model.SetError(errors.New("test error"))
				return model
			},
			action: func(model *StatusBarModel) interface{} {
				model.ClearError()
				return model.err == nil
			},
			expected: true,
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

func TestStatusBarModelView(t *testing.T) {
	startTime := time.Now()
	theme := NewDefaultTheme()
	statusBar := NewStatusBarModel(theme, startTime)

	t.Run("normal_state", func(t *testing.T) {
		view := statusBar.View()
		assert.NotEmpty(t, view, "View should not be empty")
		assert.Contains(t, view, "Session:", "Should show session info")
	})

	t.Run("loading_state", func(t *testing.T) {
		statusBar.SetLoading(true)
		view := statusBar.View()
		assert.Contains(t, view, "Thinking", "Should show thinking state when loading")
	})

	t.Run("error_state", func(t *testing.T) {
		statusBar.SetLoading(false)
		statusBar.SetError(errors.New("test error"))
		view := statusBar.View()
		assert.Contains(t, view, "Error", "Should show error when present")
		assert.Contains(t, view, "test error", "Should show error message")
	})

	t.Run("active_tool_calls", func(t *testing.T) {
		statusBar.ClearError()
		// Tool call display was removed; verify the view still renders cleanly
		view := statusBar.View()
		assert.NotEmpty(t, view, "Status bar should still render after clearing error")
	})
}

func TestStatusBarModelUpdate(t *testing.T) {
	theme := NewDefaultTheme()
	statusBar := NewStatusBarModel(theme, time.Now())

	t.Run("window_resize", func(t *testing.T) {
		resizeMsg := tea.WindowSizeMsg{Width: 100, Height: 50}
		updatedStatusBar, cmd := statusBar.Update(resizeMsg)

		assert.NotNil(t, updatedStatusBar, "Updated status bar should not be nil")
		assert.Equal(t, 100, updatedStatusBar.width, "Width should be updated")
		assert.Nil(t, cmd, "Window resize should not produce commands")
	})

	t.Run("spinner_tick_when_loading", func(t *testing.T) {
		statusBar.SetLoading(true)
		tickMsg := statusBar.spinner.Tick()
		updatedStatusBar, cmd := statusBar.Update(tickMsg)

		assert.NotNil(t, updatedStatusBar, "Updated status bar should not be nil")
		assert.NotNil(t, cmd, "Spinner tick should produce command when loading")
	})

	t.Run("spinner_tick_when_not_loading", func(t *testing.T) {
		statusBar.SetLoading(false)
		tickMsg := statusBar.spinner.Tick()
		updatedStatusBar, cmd := statusBar.Update(tickMsg)

		assert.NotNil(t, updatedStatusBar, "Updated status bar should not be nil")
		assert.Nil(t, cmd, "Spinner tick should not produce command when not loading")
	})
}

func TestStatusBarModelSpinner(t *testing.T) {
	theme := NewDefaultTheme()
	statusBar := NewStatusBarModel(theme, time.Now())

	// Test spinner command when not loading
	cmd := statusBar.GetSpinnerTickCmd()
	assert.Nil(t, cmd, "Spinner tick command should be nil when not loading")

	// Test spinner command when loading
	statusBar.SetLoading(true)
	cmd = statusBar.GetSpinnerTickCmd()
	assert.NotNil(t, cmd, "Spinner tick command should not be nil when loading")
}


func TestStatusBarModelSessionStartTime(t *testing.T) {
	startTime := time.Now().Add(-1 * time.Hour) // 1 hour ago
	theme := NewDefaultTheme()
	statusBar := NewStatusBarModel(theme, startTime)

	view := statusBar.View()
	// Should show elapsed time
	assert.Contains(t, view, "Session:", "Should show session info")
}
