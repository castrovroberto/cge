package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/castrovroberto/CGE/internal/agent"
	"github.com/castrovroberto/CGE/internal/llm"
	"github.com/castrovroberto/CGE/internal/orchestrator"
	tea "github.com/charmbracelet/bubbletea"
)

// ChatService interface abstracts the LLM/agent interaction logic
type ChatService interface {
	// SendMessage sends a message to the LLM and returns a command that will produce a response
	SendMessage(ctx context.Context, prompt string) tea.Cmd
}

// DelayProvider interface allows injecting different delay mechanisms for testing
type DelayProvider interface {
	// Delay returns a command that waits for the specified duration
	Delay(duration time.Duration) tea.Cmd
}

// HistoryService interface abstracts chat history persistence
type HistoryService interface {
	// SaveHistory saves the current chat state
	SaveHistory(sessionID, modelName string, messages []chatMessage, startTime time.Time) error
	// LoadHistory loads chat history by session ID
	LoadHistory(sessionID string) (*ChatHistory, error)
}

// Real implementations

// RealChatService provides actual LLM integration with proper function call handling
type RealChatService struct {
	llmClient    llm.Client
	toolRegistry *agent.Registry
	model        string
	systemPrompt string
}

// NewRealChatService creates a new RealChatService with the provided dependencies
func NewRealChatService(llmClient llm.Client, toolRegistry *agent.Registry, model string, systemPrompt string) *RealChatService {
	return &RealChatService{
		llmClient:    llmClient,
		toolRegistry: toolRegistry,
		model:        model,
		systemPrompt: systemPrompt,
	}
}

// SendMessage implements ChatService interface with proper function call handling
func (r *RealChatService) SendMessage(ctx context.Context, prompt string) tea.Cmd {
	return func() tea.Msg {
		// Start the chat processing asynchronously and emit tool events as they happen
		return r.startChatProcessing(ctx, prompt)
	}
}

// startChatProcessing initiates chat processing and returns a batch command
func (r *RealChatService) startChatProcessing(ctx context.Context, prompt string) tea.Msg {
	// For now, start with a simple approach that processes one iteration
	// This can be enhanced later to handle streaming tool events
	return r.processSingleChatIteration(ctx, prompt, []orchestrator.Message{
		{Role: "system", Content: r.systemPrompt},
		{Role: "user", Content: prompt},
	}, 0, time.Now())
}

// processSingleChatIteration handles one iteration of the chat loop
func (r *RealChatService) processSingleChatIteration(ctx context.Context, originalPrompt string, messages []orchestrator.Message, iteration int, startTime time.Time) tea.Msg {
	const maxIterations = 8

	if iteration >= maxIterations {
		return ollamaErrorMsg(fmt.Errorf("reached maximum chat iterations (%d)", maxIterations))
	}

	// Build prompt from message history
	prompt := r.buildPromptFromMessages(messages)

	// Prepare tool definitions
	tools := r.prepareToolDefinitions()

	// Call LLM with function calling support
	response, err := r.llmClient.GenerateWithFunctions(ctx, r.model, prompt, "", tools)
	if err != nil {
		return ollamaErrorMsg(fmt.Errorf("chat LLM call failed: %w", err))
	}

	if response.IsTextResponse {
		// Text response - this is the final answer
		duration := time.Since(startTime)
		return ollamaSuccessResponseMsg{
			response: response.TextContent,
			duration: duration,
		}
	}

	// Function call response - emit tool events and continue
	functionCall := response.FunctionCall

	// Parse parameters for display
	var params map[string]interface{}
	if err := json.Unmarshal(functionCall.Arguments, &params); err != nil {
		params = map[string]interface{}{"raw": string(functionCall.Arguments)}
	}

	// Execute the tool
	toolResult, err := r.executeTool(ctx, functionCall)

	// Create the next iteration messages
	nextMessages := append(messages, orchestrator.Message{
		Role:     "assistant",
		Content:  "",
		ToolCall: functionCall,
	})

	var resultContent string
	if err != nil {
		resultContent = fmt.Sprintf("Error executing tool: %v", err)
	} else if toolResult.Success {
		if toolResult.Data != nil {
			if dataBytes, marshalErr := json.MarshalIndent(toolResult.Data, "", "  "); marshalErr == nil {
				resultContent = string(dataBytes)
			} else {
				resultContent = fmt.Sprintf("%v", toolResult.Data)
			}
		} else {
			resultContent = "Tool executed successfully"
		}
	} else {
		resultContent = fmt.Sprintf("Tool error: %s", toolResult.Error)
	}

	// Clean the result content to remove ANSI codes and problematic characters
	resultContent = r.cleanToolOutput(resultContent)

	nextMessages = append(nextMessages, orchestrator.Message{
		Role:       "tool",
		Content:    resultContent,
		ToolCallID: functionCall.ID,
		Name:       functionCall.Name,
	})

	// Return a batch of commands: tool start, tool complete, and continue iteration
	return tea.Batch(
		func() tea.Msg {
			return toolStartMsg{
				toolCallID: functionCall.ID,
				toolName:   functionCall.Name,
				params:     params,
			}
		},
		func() tea.Msg {
			if err != nil {
				return toolCompleteMsg{
					toolCallID: functionCall.ID,
					toolName:   functionCall.Name,
					success:    false,
					result:     "",
					duration:   time.Since(startTime),
					error:      err.Error(),
				}
			}

			var resultText string
			if toolResult.Success {
				if toolResult.Data != nil {
					if dataBytes, marshalErr := json.MarshalIndent(toolResult.Data, "", "  "); marshalErr == nil {
						resultText = string(dataBytes)
					} else {
						resultText = fmt.Sprintf("%v", toolResult.Data)
					}
				} else {
					resultText = "Tool executed successfully"
				}
			} else {
				resultText = toolResult.Error
			}

			// Clean the result text to prevent ANSI codes in TUI
			resultText = r.cleanToolOutput(resultText)

			return toolCompleteMsg{
				toolCallID: functionCall.ID,
				toolName:   functionCall.Name,
				success:    toolResult.Success,
				result:     resultText,
				duration:   time.Since(startTime),
				error:      toolResult.Error,
			}
		},
		func() tea.Msg {
			// Continue with next iteration
			return r.processSingleChatIteration(ctx, originalPrompt, nextMessages, iteration+1, startTime)
		},
	)()
}

// prepareToolDefinitions converts registry tools to LLM tool definitions
func (r *RealChatService) prepareToolDefinitions() []llm.ToolDefinition {
	allTools := r.toolRegistry.List()
	definitions := make([]llm.ToolDefinition, len(allTools))

	for i, tool := range allTools {
		definitions[i] = llm.CreateToolDefinition(
			tool.Name(),
			tool.Description(),
			tool.Parameters(),
		)
	}

	return definitions
}

// buildPromptFromMessages builds a prompt string from message history
func (r *RealChatService) buildPromptFromMessages(messages []orchestrator.Message) string {
	var parts []string

	for _, msg := range messages {
		switch msg.Role {
		case "system":
			// Skip system message as it's handled separately
			continue
		case "user":
			parts = append(parts, fmt.Sprintf("User: %s", msg.Content))
		case "assistant":
			if msg.ToolCall != nil {
				parts = append(parts, fmt.Sprintf("Assistant: [Called tool: %s]", msg.ToolCall.Name))
			} else {
				parts = append(parts, fmt.Sprintf("Assistant: %s", msg.Content))
			}
		case "tool":
			parts = append(parts, fmt.Sprintf("Tool (%s): %s", msg.Name, msg.Content))
		}
	}

	return strings.Join(parts, "\n\n")
}

// executeTool executes a function call using the tool registry
func (r *RealChatService) executeTool(ctx context.Context, functionCall *llm.FunctionCall) (*agent.ToolResult, error) {
	// Look up tool in registry
	tool, exists := r.toolRegistry.Get(functionCall.Name)
	if !exists {
		return nil, fmt.Errorf("tool not found: %s", functionCall.Name)
	}

	// Validate parameters (basic JSON validation)
	var params map[string]interface{}
	if err := json.Unmarshal(functionCall.Arguments, &params); err != nil {
		return nil, fmt.Errorf("invalid tool parameters: %v", err)
	}

	// Execute tool with timeout
	toolCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	result, err := tool.Execute(toolCtx, functionCall.Arguments)
	if err != nil {
		return nil, fmt.Errorf("tool execution error: %v", err)
	}

	return result, nil
}

// stripANSI removes ANSI escape codes from text to prevent terminal formatting issues
func stripANSI(text string) string {
	// Regex to match ANSI escape codes
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return ansiRegex.ReplaceAllString(text, "")
}

// cleanToolOutput cleans tool output to remove any problematic characters
func (r *RealChatService) cleanToolOutput(text string) string {
	// Strip ANSI codes
	cleaned := stripANSI(text)

	// Remove any null bytes or other control characters
	cleaned = strings.ReplaceAll(cleaned, "\x00", "")

	// Limit length to prevent extremely long outputs from breaking the UI
	const maxLength = 2000
	if len(cleaned) > maxLength {
		cleaned = cleaned[:maxLength] + "\n... [output truncated]"
	}

	return cleaned
}

// toolStartMsg and toolCompleteMsg are used internally by RealChatService to signal
// tool lifecycle events to the Bubble Tea event loop.
type toolStartMsg struct {
	toolCallID string
	toolName   string
	params     map[string]interface{}
}

type toolCompleteMsg struct {
	toolCallID string
	toolName   string
	success    bool
	result     string
	duration   time.Duration
	error      string
}

// RealDelayProvider provides actual time delays
type RealDelayProvider struct{}

// Delay implements DelayProvider interface
func (r *RealDelayProvider) Delay(duration time.Duration) tea.Cmd {
	return tea.Tick(duration, func(t time.Time) tea.Msg {
		return delayCompleteMsg{}
	})
}

// Mock implementations for testing

// MockChatService provides controllable responses for testing
type MockChatService struct {
	Responses []MockResponse
	CallCount int
}

// MockResponse represents a predefined response for testing
type MockResponse struct {
	Response string
	Duration time.Duration
	Error    error
}

// SendMessage implements ChatService interface for testing
func (m *MockChatService) SendMessage(ctx context.Context, prompt string) tea.Cmd {
	return func() tea.Msg {
		if m.CallCount >= len(m.Responses) {
			return ollamaErrorMsg(fmt.Errorf("no more mock responses"))
		}

		resp := m.Responses[m.CallCount]
		m.CallCount++

		if resp.Error != nil {
			return ollamaErrorMsg(resp.Error)
		}

		return ollamaSuccessResponseMsg{
			response: resp.Response,
			duration: resp.Duration,
		}
	}
}

// MockDelayProvider provides controllable delays for testing
type MockDelayProvider struct {
	Delays []time.Duration
}

// Delay implements DelayProvider interface for testing
func (m *MockDelayProvider) Delay(duration time.Duration) tea.Cmd {
	// For testing, we can return immediately or track the delay
	return func() tea.Msg {
		return delayCompleteMsg{}
	}
}

// New message type for delay completion
type delayCompleteMsg struct{}
