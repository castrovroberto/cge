// internal/tui/chat/model.go
package chat

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	// Bubbles components for TUI

	tea "github.com/charmbracelet/bubbletea"

	"github.com/castrovroberto/CGE/internal/agent"
	"github.com/castrovroberto/CGE/internal/config" // Ensure this path and package are correct
	"github.com/castrovroberto/CGE/internal/llm"

	// Import the new llm package

	"github.com/castrovroberto/CGE/internal/logger" // Import logger to get the global logger
)

type (
	errMsg error
	// ollamaResponseMsg string // This line should be removed or commented out
	ollamaErrorMsg error
	// New message type to carry successful response and duration
	ollamaSuccessResponseMsg struct {
		response string
		duration time.Duration
	}
)

// Add a new message type for main context cancellation
type mainContextCancelledMsg struct{}


// chatMessage holds a single chat entry for re-rendering
type chatMessage struct {
	text         string
	isMarkdown   bool
	isCode       bool   // New: specifically for code blocks
	language     string // New: for syntax highlighting
	timestamp    time.Time
	sender       string
	placeholder  bool
	ThinkingTime time.Duration // New field for LLM thinking time

	// Enhanced tool call support
	isToolCall   bool                   // New: indicates this is a tool call message
	isToolResult bool                   // New: indicates this is a tool result message
	toolName     string                 // New: name of the tool being called/result from
	toolCallID   string                 // New: unique ID for tool call correlation
	toolSuccess  bool                   // New: whether tool execution was successful
	toolDuration time.Duration          // New: how long the tool took to execute
	toolParams   map[string]interface{} // New: tool parameters for display
}

// Add near the top after other type definitions
type chatMsgWrapper struct {
	ChatMessage
}

// Model defines the state of the chat TUI
type Model struct {
	// Component models
	theme       *Theme
	layout      *LayoutDimensions
	header      *HeaderModel
	messageList *MessageListModel
	inputArea   *InputAreaModel
	statusBar   *StatusBarModel

	// Context and config
	cfg       *config.AppConfig
	parentCtx context.Context

	// Business logic interfaces (injectable for testing)
	messageProvider MessageProvider
	delayProvider   DelayProvider
	historyService  HistoryService

	// State management
	loading           bool
	thinkingStartTime time.Time
	chatStartTime     time.Time

	// Available slash commands for suggestions
	availableCommands []string
}

var defaultSlashCommands = []string{
	"/help",
	"/model ", // Suggest space for model name
	"/clear",
	"/session ", // Suggest space for session id or action
	"/status",   // Show current status and statistics
	"/tools",    // List available tools
	"/context",  // Inject current workspace context
	"/quit",
}

// NewChatModel creates a new ChatModel using functional options
func NewChatModel(opts ...ChatModelOption) Model {
	// Initialize with default values
	m := Model{
		theme:             NewDefaultTheme(),
		availableCommands: defaultSlashCommands,
		chatStartTime:     time.Now(),
	}

	// Apply all provided options
	for _, opt := range opts {
		opt(&m)
	}

	// Ensure essential components are initialized if not provided by options
	if m.theme == nil {
		m.theme = NewDefaultTheme()
	}
	if m.layout == nil {
		m.layout = NewLayoutDimensions(m.theme)
	}

	sessionID := m.chatStartTime.Format("20060102150405")
	if m.header == nil {
		m.header = NewHeaderModel(m.theme, "Unknown", "default", sessionID, "Initializing")
	}
	if m.statusBar == nil {
		m.statusBar = NewStatusBarModel(m.theme, m.chatStartTime)
	}
	if m.inputArea == nil {
		m.inputArea = NewInputAreaModel(m.theme, m.availableCommands)
	}
	if m.messageList == nil {
		m.messageList = NewMessageListModel(m.theme, 50, 10)
	}

	// Ensure essential providers are set
	if m.messageProvider == nil {
		panic("MessageProvider is required for ChatModel")
	}
	if m.delayProvider == nil {
		m.delayProvider = &RealDelayProvider{}
	}

	// Add welcome message
	welcomeMsg := chatMessage{
		text:       "Welcome to CGE Chat! Type your message or use '/' for commands.",
		sender:     "System",
		timestamp:  time.Now(),
		isMarkdown: false,
	}
	m.messageList.AddMessage(welcomeMsg)

	return m
}

// Legacy InitialModel function for compatibility - creates ChatPresenter automatically
func InitialModel(ctx context.Context, cfg *config.AppConfig, modelName string) Model {
	// Create LLM client based on configuration
	var llmClient llm.Client
	switch cfg.LLM.Provider {
	case "ollama":
		ollamaConfig := cfg.GetOllamaConfig()
		llmClient = llm.NewOllamaClient(ollamaConfig)
	case "openai":
		openaiConfig := cfg.GetOpenAIConfig()
		llmClient = llm.NewOpenAIClient(openaiConfig)
	case "gemini":
		geminiConfig := cfg.GetGeminiConfig()
		llmClient = llm.NewGeminiClient(geminiConfig)
	default:
		// Fallback to ollama if provider is unknown
		ollamaConfig := cfg.GetOllamaConfig()
		llmClient = llm.NewOllamaClient(ollamaConfig)
	}

	// Create tool registry with chat tools
	workspaceRoot := cfg.Project.WorkspaceRoot
	if workspaceRoot == "" {
		workspaceRoot = "." // Fallback to current directory
	}

	// Convert workspace root to absolute path to fix tool access issues
	absWorkspaceRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		absWorkspaceRoot = workspaceRoot
	}

	toolFactory := agent.NewToolFactory(absWorkspaceRoot)
	toolRegistry := toolFactory.CreateGenerationRegistry()

	// Create header model to get current context
	sessionID := time.Now().Format("20060102150405")
	headerModel := NewHeaderModel(NewDefaultTheme(), cfg.LLM.Provider, modelName, sessionID, "Active")

	// Get the base system prompt
	systemPrompt := cfg.GetLoadedChatSystemPrompt()

	// Add header context to the system prompt
	headerContext := headerModel.FormatContextForLLM()
	enhancedSystemPrompt := headerContext + "\n" + systemPrompt + "\n\n" + buildContextAwarenessInstructions(absWorkspaceRoot)

	presenter := NewChatPresenter(ctx, llmClient, toolRegistry, enhancedSystemPrompt, modelName)

	// Create model with options including the header model
	model := NewChatModel(
		WithParentContext(ctx),
		WithInitialConfig(cfg),
		WithMessageProvider(presenter),
		WithDelayProvider(&RealDelayProvider{}),
		WithHeader(headerModel), // Pass the header model to the chat model
	)

	return model
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.statusBar.GetSpinnerTickCmd(),
		m.listenForMessages(), // Start listening for messages from the provider
	)
}

func (m Model) sendMessage(prompt string) tea.Cmd {
	// Send the message through the message provider
	if err := m.messageProvider.Send(m.parentCtx, prompt); err != nil {
		return func() tea.Msg {
			return errMsg(err)
		}
	}
	return nil
}

// listenForMessages creates a command that listens to the message provider's channel
func (m Model) listenForMessages() tea.Cmd {
	return func() tea.Msg {
		select {
		case msg, ok := <-m.messageProvider.Messages():
			if !ok {
				return ProviderClosedMessage{}
			}
			return chatMsgWrapper{ChatMessage: msg}
		case <-m.parentCtx.Done():
			return mainContextCancelledMsg{}
		}
	}
}

// convertToTuiMessage converts a ChatMessage to the internal chatMessage format
func convertToTuiMessage(msg ChatMessage) chatMessage {
	tuiMsg := chatMessage{
		text:      msg.Text,
		sender:    msg.Sender,
		timestamp: msg.Timestamp,
	}

	// Map message types to appropriate display properties
	switch msg.Type {
	case AssistantMessage:
		tuiMsg.isMarkdown = true
	case ToolCallMessage:
		tuiMsg.isToolCall = true
		if name, ok := msg.Metadata["tool_name"].(string); ok {
			tuiMsg.toolName = name
		}
		if id, ok := msg.Metadata["tool_call_id"].(string); ok {
			tuiMsg.toolCallID = id
		}
		if params, ok := msg.Metadata["params"].(map[string]interface{}); ok {
			tuiMsg.toolParams = params
		}
	case ToolResultMessage:
		tuiMsg.isToolResult = true
		if name, ok := msg.Metadata["tool_name"].(string); ok {
			tuiMsg.toolName = name
		}
		if id, ok := msg.Metadata["tool_call_id"].(string); ok {
			tuiMsg.toolCallID = id
		}
		if success, ok := msg.Metadata["success"].(bool); ok {
			tuiMsg.toolSuccess = success
		}
		if duration, ok := msg.Metadata["duration"].(time.Duration); ok {
			tuiMsg.toolDuration = duration
		}
	case ErrorMessage:
		// Error messages are displayed as regular text, but could be styled differently
	}

	return tuiMsg
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	// Handle main model logic
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Update all components that need width/height information
		var headerCmd, inputCmd, statusBarCmd, msgListCmd tea.Cmd

		// Update header first to ensure height calculation is current
		m.header, headerCmd = m.header.Update(msg)
		if headerCmd != nil {
			cmds = append(cmds, headerCmd)
		}

		m.statusBar, statusBarCmd = m.statusBar.Update(msg)
		if statusBarCmd != nil {
			cmds = append(cmds, statusBarCmd)
		}

		// Update input area to get its current height after resize
		m.inputArea, inputCmd = m.inputArea.Update(msg)
		if inputCmd != nil {
			cmds = append(cmds, inputCmd)
		}

		// Calculate viewport height using centralized layout dimensions with dynamic header
		textareaHeight := m.inputArea.GetHeight()
		suggestionAreaHeight := m.inputArea.GetSuggestionAreaHeight()

		// Use the new header-aware layout validation
		err := m.layout.ValidateLayoutWithHeader(
			msg.Height,
			textareaHeight,
			suggestionAreaHeight,
			m.layout.GetViewportFrameHeight(),
			m.header,
		)
		if err != nil {
			logger.Get().Warn("Layout validation failed", "error", err)
			// In case of layout validation failure, try to use a safe fallback
			logger.Get().Debug("Layout validation details",
				"windowHeight", msg.Height,
				"headerHeight", m.header.GetHeight(),
				"textareaHeight", textareaHeight,
				"suggestionAreaHeight", suggestionAreaHeight,
				"statusBarHeight", m.layout.GetStatusBarHeight(),
				"viewportFrameHeight", m.layout.GetViewportFrameHeight())
		}

		// Add debug layout information with dynamic header height
		m.debugLayoutInfoWithHeader(msg.Width, msg.Height, textareaHeight, suggestionAreaHeight)

		viewportHeight := m.layout.CalculateViewportHeightWithHeader(
			msg.Height,
			textareaHeight,
			suggestionAreaHeight,
			m.layout.GetViewportFrameHeight(),
			m.header,
		)

		// Ensure viewport height is reasonable
		if viewportHeight < m.layout.GetMinViewportHeight() {
			logger.Get().Warn("Calculated viewport height is too small, using minimum",
				"calculated", viewportHeight,
				"minimum", m.layout.GetMinViewportHeight())
			viewportHeight = m.layout.GetMinViewportHeight()
		}

		m.messageList.SetWidth(msg.Width)
		m.messageList.SetHeight(viewportHeight)
		m.messageList, msgListCmd = m.messageList.Update(msg)
		if msgListCmd != nil {
			cmds = append(cmds, msgListCmd)
		}

	case tea.KeyMsg:
		// Handle key messages
		switch msg.String() {
		case "ctrl+c":
			logger.Get().Info("Ctrl+C pressed, attempting to save chat history and quit TUI.")
			if err := m.SaveHistory(); err != nil {
				m.statusBar.SetError(fmt.Errorf("error saving history on Ctrl+C: %w", err))
				logger.Get().Error("Failed to save chat history on Ctrl+C", "error", err)
			} else {
				logger.Get().Info("Chat history saved successfully on Ctrl+C.")
			}
			return m, tea.Quit

		case "tab":
			if m.inputArea.ApplySelectedSuggestion() {
				// Suggestion was applied, don't pass to input area
				return m, nil
			}
			// No suggestion to apply, let input area handle it
			m.inputArea, cmd = m.inputArea.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}

		case "escape":
			if m.inputArea.HasSuggestions() {
				m.inputArea.ClearSuggestions()
				return m, nil
			}
			// No suggestions to clear, handle as normal escape
			m.inputArea, cmd = m.inputArea.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}

		case "up":
			if m.inputArea.HandleSuggestionNavigation("up") {
				// Navigation handled by input area
				return m, nil
			}
			// No suggestions, let input area handle normally
			m.inputArea, cmd = m.inputArea.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}

		case "down":
			if m.inputArea.HandleSuggestionNavigation("down") {
				// Navigation handled by input area
				return m, nil
			}
			// No suggestions, let input area handle normally
			m.inputArea, cmd = m.inputArea.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}

		case "enter":
			// If suggestions are active and one is selected, apply it first
			if m.inputArea.ApplySelectedSuggestion() {
				// Suggestion was applied, don't send message yet
				return m, nil
			}

			if m.inputArea.GetValue() != "" && !m.loading {
				// Check if we should refresh context before sending message
				if m.ShouldRefreshContext() {
					m.RefreshWorkspaceContext()
				}

				userPrompt := m.inputArea.GetValue()

				// Handle special slash commands
				if strings.HasPrefix(userPrompt, "/context") {
					m.InjectCurrentContext()
					m.inputArea.Reset()
					return m, nil
				}

				// Start loading state with proper coordination
				m.setLoading(true)

				m.messageList.AddMessage(chatMessage{
					text:      userPrompt,
					sender:    "You",
					timestamp: time.Now(),
				})

				// Add a placeholder for the assistant response
				m.messageList.AddMessage(chatMessage{
					text:        "Thinking...",
					sender:      "Assistant",
					timestamp:   time.Now(),
					placeholder: true,
				})

				m.inputArea.Reset()
				return m, tea.Batch(m.sendMessage(userPrompt), m.statusBar.GetSpinnerTickCmd())
			}

		default:
			// Pass other keys to input area
			m.inputArea, cmd = m.inputArea.Update(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case ollamaSuccessResponseMsg:
		// End loading state with proper coordination and cleanup
		m.setLoading(false)
		m.statusBar.ClearError()

		// Calculate thinking time from the start time
		var thinkingTime time.Duration
		if !m.thinkingStartTime.IsZero() {
			thinkingTime = msg.duration
		}

		responseMsg := chatMessage{
			text:         msg.response,
			sender:       "Assistant",
			timestamp:    time.Now(),
			isMarkdown:   true,
			ThinkingTime: thinkingTime,
		}
		m.messageList.ReplacePlaceholder(responseMsg)

		// Reset thinking start time
		m.thinkingStartTime = time.Time{}

	case ollamaErrorMsg:
		// End loading state and handle error
		m.setError(msg)

		errorMsg := chatMessage{
			text:      fmt.Sprintf("Error: %v", msg),
			sender:    "System",
			timestamp: time.Now(),
		}
		m.messageList.ReplacePlaceholder(errorMsg)

		// Reset thinking start time
		m.thinkingStartTime = time.Time{}

	case errMsg:
		m.setError(msg)

	case TurnCompleteMessage:
		// Presenter finished a turn; ensure loading is cleared regardless of what messages arrived.
		m.setLoading(false)
		return m, m.listenForMessages()

	case ProviderClosedMessage:
		// Channel was closed intentionally (e.g. on quit); nothing to display.
		return m, nil

	case chatMsgWrapper:
		// Handle new messages from the MessageProvider
		chatMessage := msg.ChatMessage
		switch chatMessage.Type {
		case UserMessage:
			// User messages are typically added when sending, but could be echoed back
			m.messageList.AddMessage(convertToTuiMessage(chatMessage))
		case AssistantMessage:
			m.setLoading(false) // Stop loading when we receive assistant response
			m.messageList.ReplacePlaceholder(convertToTuiMessage(chatMessage))
		case ErrorMessage:
			m.setError(fmt.Errorf("%s", chatMessage.Text))
			m.messageList.ReplacePlaceholder(convertToTuiMessage(chatMessage))
		case ToolCallMessage:
			m.messageList.AddMessage(convertToTuiMessage(chatMessage))
		case ToolResultMessage:
			m.messageList.AddMessage(convertToTuiMessage(chatMessage))
		case SystemMessage:
			m.messageList.AddMessage(convertToTuiMessage(chatMessage))
		case TurnComplete:
			// Handled via TurnCompleteMessage above; ignore if it arrives wrapped.
		}
		m.messageList.GotoBottom()
		// Return a new command to continue listening
		return m, m.listenForMessages()

	default:
		// Update input area for other messages
		m.inputArea, cmd = m.inputArea.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m Model) View() string {
	return strings.Join([]string{
		m.header.View(),
		m.messageList.View(),
		m.inputArea.View(),
		m.statusBar.View(),
	}, "\n")
}

// LoadHistory loads a previous chat history into the model
func (m *Model) LoadHistory(history *ChatHistory) {
	m.header.SetSessionID(history.SessionID)
	m.header.SetModelName(history.ModelName)
	m.messageList.LoadHistory(history.Messages)
}

func formatToolDescriptions(tools []map[string]interface{}) string {
	var sb strings.Builder
	for _, tool := range tools {
		fmt.Fprintf(&sb, "\n%s: %s\nParameters: %s\n",
			tool["name"], tool["description"], tool["parameters"])
	}
	return sb.String()
}

// State management helper methods

// setLoading sets the loading state consistently across components
func (m *Model) setLoading(loading bool) {
	m.loading = loading
	if loading {
		m.thinkingStartTime = time.Now()
	}
	m.statusBar.SetLoading(loading)
}

// setError sets error state and clears loading
func (m *Model) setError(err error) {
	m.loading = false
	m.statusBar.SetLoading(false)
	m.statusBar.SetError(err)
}


// debugLayoutInfo logs detailed layout information for troubleshooting
func (m *Model) debugLayoutInfo(windowWidth, windowHeight, textareaHeight, suggestionAreaHeight int) {
	logger.Get().Debug("Layout debug info",
		"windowWidth", windowWidth,
		"windowHeight", windowHeight,
		"headerHeight", m.layout.GetHeaderHeight(),
		"statusBarHeight", m.layout.GetStatusBarHeight(),
		"inputAreaHeight", textareaHeight,
		"suggestionAreaHeight", suggestionAreaHeight,
		"viewportFrameHeight", m.layout.GetViewportFrameHeight(),
		"calculatedViewportHeight", m.layout.CalculateViewportHeight(windowHeight, textareaHeight, suggestionAreaHeight, m.layout.GetViewportFrameHeight()),
	)
}

// debugLayoutInfoWithHeader logs detailed layout information for troubleshooting with dynamic header height
func (m *Model) debugLayoutInfoWithHeader(windowWidth, windowHeight, textareaHeight, suggestionAreaHeight int) {
	logger.Get().Debug("Layout debug info with dynamic header height",
		"windowWidth", windowWidth,
		"windowHeight", windowHeight,
		"headerHeight", m.header.GetHeight(),
		"statusBarHeight", m.layout.GetStatusBarHeight(),
		"inputAreaHeight", textareaHeight,
		"suggestionAreaHeight", suggestionAreaHeight,
		"viewportFrameHeight", m.layout.GetViewportFrameHeight(),
		"calculatedViewportHeight", m.layout.CalculateViewportHeightWithHeader(windowHeight, textareaHeight, suggestionAreaHeight, m.layout.GetViewportFrameHeight(), m.header),
	)
}

// Getter methods for accessing model components

// Header returns the header model
func (m *Model) Header() *HeaderModel {
	return m.header
}

// MessageList returns the message list model
func (m *Model) MessageList() *MessageListModel {
	return m.messageList
}

// InputArea returns the input area model
func (m *Model) InputArea() *InputAreaModel {
	return m.inputArea
}

// StatusBar returns the status bar model
func (m *Model) StatusBar() *StatusBarModel {
	return m.statusBar
}

// Theme returns the theme
func (m *Model) Theme() *Theme {
	return m.theme
}

// RefreshWorkspaceContext refreshes the workspace context and optionally injects it into the conversation
func (m *Model) RefreshWorkspaceContext() {
	if m.header != nil {
		// Refresh Git information in the header
		m.header.RefreshGitInfo()

		// Add context refresh message to the conversation
		contextMsg := chatMessage{
			text:       "🔄 Workspace context refreshed",
			sender:     "System",
			timestamp:  time.Now(),
			isMarkdown: false,
		}
		m.messageList.AddMessage(contextMsg)
	}
}

// InjectCurrentContext injects the current workspace context into the conversation
func (m *Model) InjectCurrentContext() {
	if m.header != nil {
		contextText := m.header.FormatContextForLLM()
		contextMsg := chatMessage{
			text:       contextText,
			sender:     "System",
			timestamp:  time.Now(),
			isMarkdown: true,
		}
		m.messageList.AddMessage(contextMsg)
	}
}

// ShouldRefreshContext determines if context should be refreshed based on conversation length
func (m *Model) ShouldRefreshContext() bool {
	n := len(m.messageList.GetMessages())
	return m.messageList != nil && n > 0 && n%10 == 0
}

// buildContextAwarenessInstructions creates enhanced context instructions for the LLM
func buildContextAwarenessInstructions(workspaceRoot string) string {
	return fmt.Sprintf(`
## 🔍 Environment Context Instructions

You are operating in the following environment:
- **Working Directory**: %s
- **Context Tools Available**: git_info, list_directory, codebase_search

### Context Gathering Protocol
Before starting any complex task:
1. Use git_info to understand the current Git branch and repository status
2. Use list_directory to explore the project structure and understand the codebase layout
3. Consider the project type and existing patterns before making recommendations

### Workspace Awareness
- All file paths should be interpreted relative to the workspace root
- Check Git status before making changes that might conflict with existing work
- Understand the project structure before suggesting new files or directories
- Follow existing code patterns and conventions evident in the codebase

### Enhanced Decision Making
Factor in:
- Current Git branch and any uncommitted changes
- Project structure and organization patterns
- Existing dependencies and frameworks in use
- File naming and directory organization conventions

Use the context tools proactively to provide more relevant and contextually-aware assistance.
`, workspaceRoot)
}
