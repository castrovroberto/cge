package agent

import (
	"fmt"
)

// ToolFactoryConfig holds configuration for all tools that need it
type ToolFactoryConfig struct {
	ListDirectory *ListDirToolConfig
	// Future tool configs can be added here
	// ShellRun      *ShellRunToolConfig
	// Git           *GitToolConfig
}

// ToolFactory creates and configures tool registries
type ToolFactory struct {
	workspaceRoot string
	config        *ToolFactoryConfig
}

// NewToolFactory creates a new tool factory with default configuration
func NewToolFactory(workspaceRoot string) *ToolFactory {
	return &ToolFactory{
		workspaceRoot: workspaceRoot,
		config:        &ToolFactoryConfig{}, // Empty config uses tool defaults
	}
}

// NewToolFactoryWithConfig creates a new tool factory with custom configuration
func NewToolFactoryWithConfig(workspaceRoot string, config ToolFactoryConfig) *ToolFactory {
	return &ToolFactory{
		workspaceRoot: workspaceRoot,
		config:        &config,
	}
}

// SetListDirectoryConfig updates the list directory configuration
func (tf *ToolFactory) SetListDirectoryConfig(config ListDirToolConfig) {
	if tf.config == nil {
		tf.config = &ToolFactoryConfig{}
	}
	tf.config.ListDirectory = &config
}

// CreateRegistry creates a new registry with all available tools
func (tf *ToolFactory) CreateRegistry() *Registry {
	registry := NewRegistry()

	// Register all available tools
	tf.registerCoreTool(registry)

	return registry
}

// CreatePlanningRegistry creates a registry with tools suitable for planning
func (tf *ToolFactory) CreatePlanningRegistry() *Registry {
	registry := NewRegistry()

	// Planning tools - read-only operations
	registry.Register(NewFileReadTool(tf.workspaceRoot))
	registry.Register(NewCodeSearchTool(tf.workspaceRoot))
	registry.Register(tf.createListDirTool())
	registry.Register(NewGitTool(tf.workspaceRoot))
	// Add clarification tool for planning when uncertainty arises
	registry.Register(NewClarificationTool(tf.workspaceRoot))

	return registry
}

// CreateGenerationRegistry creates a registry with tools suitable for code generation
func (tf *ToolFactory) CreateGenerationRegistry() *Registry {
	registry := NewRegistry()

	// Generation tools - read and write operations
	registry.Register(NewFileReadTool(tf.workspaceRoot))
	registry.Register(NewFileWriteTool(tf.workspaceRoot))
	registry.Register(NewCodeSearchTool(tf.workspaceRoot))
	registry.Register(tf.createListDirTool())
	registry.Register(NewPatchApplyTool(tf.workspaceRoot))
	registry.Register(NewGitTool(tf.workspaceRoot))
	// Add clarification tool for generation when requirements are unclear
	registry.Register(NewClarificationTool(tf.workspaceRoot))

	return registry
}

// CreateReviewRegistry creates a registry with tools suitable for code review
func (tf *ToolFactory) CreateReviewRegistry() *Registry {
	registry := NewRegistry()

	// Review tools - read, write, and test operations
	registry.Register(NewFileReadTool(tf.workspaceRoot))
	registry.Register(NewFileWriteTool(tf.workspaceRoot))
	registry.Register(NewCodeSearchTool(tf.workspaceRoot))
	registry.Register(tf.createListDirTool())
	registry.Register(NewPatchApplyTool(tf.workspaceRoot))
	registry.Register(NewShellRunTool(tf.workspaceRoot))
	registry.Register(NewGitTool(tf.workspaceRoot))
	registry.Register(NewGitCommitTool(tf.workspaceRoot))
	registry.Register(NewTestRunnerTool(tf.workspaceRoot))
	registry.Register(NewLintRunnerTool(tf.workspaceRoot))
	registry.Register(NewParseTestResultsTool(tf.workspaceRoot))
	registry.Register(NewParseLintResultsTool(tf.workspaceRoot))
	// Add clarification tool for review when fixes are ambiguous
	registry.Register(NewClarificationTool(tf.workspaceRoot))

	return registry
}

// CreateChatRegistry creates a registry with tools suitable for interactive chat sessions
func (tf *ToolFactory) CreateChatRegistry() *Registry {
	registry := NewRegistry()

	// Chat tools - includes AI-powered commit tool
	registry.Register(NewFileReadTool(tf.workspaceRoot))
	registry.Register(NewFileWriteTool(tf.workspaceRoot))
	registry.Register(NewCodeSearchTool(tf.workspaceRoot))
	registry.Register(tf.createListDirTool())
	registry.Register(NewPatchApplyTool(tf.workspaceRoot))
	registry.Register(NewShellRunTool(tf.workspaceRoot))
	registry.Register(NewGitTool(tf.workspaceRoot))
	registry.Register(NewGitCommitTool(tf.workspaceRoot))
	// Note: AI commit tool will be added separately due to dependency injection needs
	registry.Register(NewTestRunnerTool(tf.workspaceRoot))
	registry.Register(NewLintRunnerTool(tf.workspaceRoot))
	registry.Register(NewParseTestResultsTool(tf.workspaceRoot))
	registry.Register(NewParseLintResultsTool(tf.workspaceRoot))
	registry.Register(NewClarificationTool(tf.workspaceRoot))

	return registry
}

// CreateFullRegistry creates a registry with all available tools
func (tf *ToolFactory) CreateFullRegistry() *Registry {
	registry := NewRegistry()
	tf.registerCoreTool(registry)
	return registry
}

// registerCoreTool registers all core tools
func (tf *ToolFactory) registerCoreTool(registry *Registry) {
	tools := []Tool{
		NewFileReadTool(tf.workspaceRoot),
		NewFileWriteTool(tf.workspaceRoot),
		NewCodeSearchTool(tf.workspaceRoot),
		tf.createListDirTool(),
		NewPatchApplyTool(tf.workspaceRoot),
		NewShellRunTool(tf.workspaceRoot),
		NewGitTool(tf.workspaceRoot),
		NewGitCommitTool(tf.workspaceRoot),
		NewTestRunnerTool(tf.workspaceRoot),
		NewLintRunnerTool(tf.workspaceRoot),
		NewParseTestResultsTool(tf.workspaceRoot),
		NewParseLintResultsTool(tf.workspaceRoot),
		NewClarificationTool(tf.workspaceRoot),
	}

	for _, tool := range tools {
		if err := registry.Register(tool); err != nil {
			// Log error but continue - this shouldn't happen with our tools
			fmt.Printf("Warning: failed to register tool %s: %v\n", tool.Name(), err)
		}
	}
}

// createListDirTool creates the appropriate list directory tool based on configuration
func (tf *ToolFactory) createListDirTool() Tool {
	if tf.config != nil && tf.config.ListDirectory != nil {
		return NewListDirToolWithConfig(tf.workspaceRoot, *tf.config.ListDirectory)
	}
	return NewListDirTool(tf.workspaceRoot)
}

// GetAvailableToolNames returns the names of all available tools
func (tf *ToolFactory) GetAvailableToolNames() []string {
	return []string{
		"read_file",
		"write_file",
		"codebase_search",
		"list_directory",
		"apply_patch_to_file",
		"run_shell_command",
		"git_info",
		"git_commit",
		"run_tests",
		"run_linter",
		"parse_test_results",
		"parse_lint_results",
		"request_human_clarification",
	}
}
