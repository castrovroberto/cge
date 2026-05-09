# Context Awareness Fixes

This document outlines the comprehensive fixes implemented to address the disconnect between the visual context shown in the TUI header and the LLM's programmatic awareness of that context.

## Problem Analysis

The original issue was that while the TUI header displayed crucial contextual information (working directory, Git branch, etc.), this information was not being programmatically passed to the LLM, leading to responses where the assistant would ask for codebase paths or locations despite having this information visually available.

## Implemented Solutions

### 1. Enhanced System Prompt with Explicit Instructions

**File:** `system-prompt.md`

**Changes:**
- Added **CRITICAL: Proactive Context Gathering** section
- Explicit instructions to use `git_info` and `list_directory` tools immediately
- Clear prohibition against asking for codebase paths
- **IMPORTANT REMINDERS** section emphasizing key points

**Key Additions:**
```markdown
## CRITICAL: Proactive Context Gathering
**BEFORE responding to ANY user request, you MUST:**

1. **Immediately use `git_info`** to understand the current Git repository status and branch
2. **Use `list_directory` with the root directory (".")** to explore the project structure
3. **Analyze the project type** from configuration files (go.mod, package.json, etc.)
4. **Consider the working directory** shown in your session context

**Do NOT ask the user for the codebase path or location** - use the tools to discover this information yourself.
```

### 2. Header Context Extraction and Formatting

**File:** `internal/tui/chat/header_model.go`

**New Methods:**
- `GetWorkspaceContext()`: Returns structured workspace context information
- `FormatContextForLLM()`: Formats header context for inclusion in LLM prompts
- `RefreshGitInfo()`: Updates Git information for periodic refresh

**Context Structure:**
```go
type WorkspaceContextInfo struct {
    WorkingDirectory string `json:"working_directory"`
    GitBranch       string `json:"git_branch"`
    GitRepository   bool   `json:"git_repository"`
    Provider        string `json:"provider"`
    ModelName       string `json:"model_name"`
    SessionID       string `json:"session_id"`
    SessionUUID     string `json:"session_uuid"`
}
```

### 3. Automatic Context Injection in Chat Flow

**File:** `internal/tui/chat/model.go`

**Enhanced InitialModel Function:**
- Creates header model with current context
- Extracts formatted context from header
- Prepends header context to system prompt
- Ensures LLM receives workspace context from the start

**New Methods:**
- `RefreshWorkspaceContext()`: Refreshes and injects updated context
- `InjectCurrentContext()`: Manually injects current context into conversation
- `ShouldRefreshContext()`: Determines when context refresh is needed

### 4. Periodic Context Refresh

**Implementation:**
- Automatic context refresh every 10 messages to prevent context window loss
- Manual `/context` slash command for user-initiated context injection
- Git information refresh when needed

### 5. Enhanced Chat Model Options

**File:** `internal/tui/chat/model_options.go`

**New Option:**
- `WithHeader(header *HeaderModel)`: Allows passing header model to chat model

## Technical Flow

### 1. Session Initialization
```go
// Create header model with current context
headerModel := NewHeaderModel(theme, provider, modelName, sessionID, "Active")

// Extract formatted context
headerContext := headerModel.FormatContextForLLM()

// Enhance system prompt with context
enhancedSystemPrompt := headerContext + "\n" + systemPrompt + "\n\n" + contextInstructions
```

### 2. Context Injection Format
```markdown
## 🔍 Current Session Context

**Working Directory:** `~/dev/cge`
**Absolute Path:** `/Users/user/dev/cge`
**Git Repository:** Yes
**Current Branch:** `refactor/cge`
**LLM Provider:** ollama
**Model:** llama3.2:latest

**Available Context Tools:**
- `git_info`: Get detailed Git repository information
- `list_directory`: Explore project structure and files
- `codebase_search`: Search for code patterns and content

**Instructions:**
- Use the context tools proactively to understand the project structure
- Check Git status before making changes
- Explore the directory structure to understand the codebase
- All file paths are relative to the working directory shown above
```

### 3. Automatic Context Refresh
```go
// In message handling
if m.ShouldRefreshContext() {
    m.RefreshWorkspaceContext()
}
```

### 4. Manual Context Commands
- `/context` command injects current workspace context
- Added to default slash commands list

## Benefits

### 1. Immediate Context Awareness
- LLM receives workspace context from the first message
- No more asking for codebase paths or locations
- Understands working directory and Git context immediately

### 2. Proactive Tool Usage
- System prompt explicitly instructs to use context tools
- Clear protocol for gathering information before responding
- Prevents assumptions and guesswork

### 3. Context Persistence
- Periodic refresh prevents context loss in long conversations
- Manual refresh capability via slash commands
- Git information stays current

### 4. Enhanced Decision Making
- LLM considers Git branch and status
- Understands project structure from the start
- Respects existing patterns and conventions

## Usage Examples

### Before Fixes
```
User: "Help me understand this project structure"
Assistant: "I'd need the path to the codebase or some sample code to help you understand the project structure."
```

### After Fixes
```
User: "Help me understand this project structure"
Assistant: [Uses git_info and list_directory tools immediately]
"I can see you're working in the CGE project in `/Users/user/dev/cge` on the `refactor/cge` branch. Let me explore the structure..."
```

## Testing the Fixes

### 1. Build and Run
```bash
go build -o CGE .
./CGE chat
```

### 2. Test Context Injection
- Start chat session
- Type `/context` to manually inject context
- Observe that LLM now has workspace information

### 3. Verify Proactive Tool Usage
- Ask questions about the project
- Verify LLM uses `git_info` and `list_directory` proactively
- Confirm no requests for codebase paths

## Integration Points

### 1. System Prompt Enhancement
- Clear, explicit instructions for context gathering
- Prohibition against asking for known information
- Step-by-step context gathering protocol

### 2. Chat Flow Integration
- Context injection at session start
- Periodic refresh during conversation
- Manual refresh capability

### 3. Header-Chat Bridge
- Header context extraction methods
- Formatted context for LLM consumption
- Git information refresh

## Future Enhancements

### 1. Real-time Context Updates
- File system change monitoring
- Automatic Git status updates
- Dynamic context refresh

### 2. Enhanced Context Intelligence
- Project type-specific context
- Dependency-aware context
- Activity-based context prioritization

### 3. Context Optimization
- Smart context window management
- Context relevance scoring
- Adaptive refresh timing

## Conclusion

These fixes comprehensively address the context awareness gap by:

1. **Ensuring immediate context availability** through header integration
2. **Mandating proactive tool usage** through enhanced system prompts
3. **Maintaining context currency** through periodic refresh
4. **Providing manual control** through slash commands

The LLM now has full awareness of its operational environment from the start of every session, eliminating the disconnect between visual context and programmatic awareness. 