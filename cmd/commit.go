package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/castrovroberto/CGE/internal/config"
	"github.com/castrovroberto/CGE/internal/contextkeys"
	"github.com/castrovroberto/CGE/internal/llm"
	"github.com/castrovroberto/CGE/internal/templates"
	"github.com/spf13/cobra"
)

var (
	commitDryRun      bool
	commitAutoCommit  bool
	commitType        string
	commitScope       string
	commitBreaking    bool
	commitInteractive bool
	commitFiles       []string
)

// commitCmd represents the commit command
var commitCmd = &cobra.Command{
	Use:   "commit",
	Short: "Generate intelligent commit messages and optionally commit changes",
	Long: `The commit command analyzes your git changes and generates intelligent,
conventional commit messages using AI. It can:

- Analyze staged and unstaged changes
- Generate conventional commit messages (feat, fix, docs, etc.)
- Show diffs and summaries of changes
- Optionally commit the changes automatically
- Support interactive mode for reviewing generated messages

Examples:
  CGE commit                           # Generate message for all changes
  CGE commit --dry-run                 # Preview message without committing
  CGE commit --auto-commit             # Generate and commit automatically
  CGE commit --type feat --scope auth  # Specify commit type and scope
  CGE commit --files main.go utils.go  # Generate message for specific files
  CGE commit --interactive             # Interactive mode with review options`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		logger := contextkeys.LoggerFromContext(ctx)
		cfg := contextkeys.ConfigFromContext(ctx)

		logger.Info("Analyzing git changes for commit message generation...")

		// Get workspace root
		workspaceRoot := cfg.Project.WorkspaceRoot
		if workspaceRoot == "" {
			var err error
			workspaceRoot, err = os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}
		}

		// Convert to absolute path
		absWorkspaceRoot, err := filepath.Abs(workspaceRoot)
		if err != nil {
			return fmt.Errorf("failed to convert workspace root to absolute path: %w", err)
		}

		// Check if this is a git repository
		if !isGitRepo(absWorkspaceRoot) {
			return fmt.Errorf("not a git repository")
		}

		// Initialize LLM client
		var llmClient llm.Client
		switch cfg.LLM.Provider {
		case "ollama":
			ollamaConfig := cfg.GetOllamaConfig()
			llmClient = llm.NewOllamaClient(ollamaConfig)
			logger.Info("Using Ollama client for commit message generation", "host", ollamaConfig.HostURL)
		case "openai":
			openaiConfig := cfg.GetOpenAIConfig()
			llmClient = llm.NewOpenAIClient(openaiConfig)
			logger.Info("Using OpenAI client for commit message generation", "base_url", openaiConfig.BaseURL)
		case "gemini":
			geminiConfig := cfg.GetGeminiConfig()
			llmClient = llm.NewGeminiClient(geminiConfig)
			logger.Info("Using Gemini client for commit message generation", "model", cfg.LLM.Model)
		default:
			return fmt.Errorf("unsupported LLM provider: %s", cfg.LLM.Provider)
		}

		// Initialize template engine
		promptsDir := filepath.Join(absWorkspaceRoot, "prompts")
		templateEngine := templates.NewEngine(promptsDir)

		// Get git changes
		changes, err := analyzeGitChanges(absWorkspaceRoot, commitFiles)
		if err != nil {
			return fmt.Errorf("failed to analyze git changes: %w", err)
		}

		if len(changes.StagedFiles) == 0 && len(changes.UnstagedFiles) == 0 {
			logger.Info("No changes found to commit")
			return nil
		}

		// Generate commit message
		commitMessage, err := generateCommitMessage(ctx, llmClient, templateEngine, changes, CommitOptions{
			Type:        commitType,
			Scope:       commitScope,
			Breaking:    commitBreaking,
			Interactive: commitInteractive,
		}, &cfg)
		if err != nil {
			return fmt.Errorf("failed to generate commit message: %w", err)
		}

		// Display the generated commit message
		fmt.Printf("\n=== Generated Commit Message ===\n")
		fmt.Printf("%s\n", commitMessage)

		if commitDryRun {
			fmt.Printf("\nDRY RUN: Would commit the following changes:\n")
			printChangeSummary(changes)
			return nil
		}

		if commitInteractive {
			if !confirmCommit(commitMessage, changes) {
				fmt.Println("Commit cancelled by user.")
				return nil
			}
		}

		if commitAutoCommit || commitInteractive {
			// Stage files if not already staged
			if len(changes.UnstagedFiles) > 0 {
				if err := stageFiles(absWorkspaceRoot, changes.UnstagedFiles); err != nil {
					return fmt.Errorf("failed to stage files: %w", err)
				}
			}

			// Create the commit
			commitHash, err := createCommit(absWorkspaceRoot, commitMessage)
			if err != nil {
				return fmt.Errorf("failed to create commit: %w", err)
			}

			fmt.Printf("\n✅ Successfully committed changes: %s\n", commitHash)
			logger.Info("Successfully created commit", "hash", commitHash, "message", commitMessage)
		} else {
			fmt.Printf("\nTo commit these changes, run:\n")
			fmt.Printf("git add . && git commit -m \"%s\"\n", strings.ReplaceAll(commitMessage, "\"", "\\\""))
		}

		return nil
	},
}

// GitChanges represents the analysis of git repository changes
type GitChanges struct {
	StagedFiles   []FileChange `json:"staged_files"`
	UnstagedFiles []FileChange `json:"unstaged_files"`
	CurrentBranch string       `json:"current_branch"`
	RecentCommits []Commit     `json:"recent_commits"`
}

// FileChange represents a change to a single file
type FileChange struct {
	FilePath   string `json:"file_path"`
	Status     string `json:"status"` // A, M, D, R, C, U, ?
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	DiffSample string `json:"diff_sample"` // First few lines of diff for context
}

// Commit represents a git commit
type Commit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Message string `json:"message"`
	Date    string `json:"date"`
}

// CommitOptions represents options for commit message generation
type CommitOptions struct {
	Type        string
	Scope       string
	Breaking    bool
	Interactive bool
}

// analyzeGitChanges analyzes the current git repository state
func analyzeGitChanges(workspaceRoot string, specificFiles []string) (*GitChanges, error) {
	changes := &GitChanges{}

	// Get current branch
	branch, err := getCurrentBranch(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to get current branch: %w", err)
	}
	changes.CurrentBranch = branch

	// Get recent commits for context
	commits, err := getRecentCommits(workspaceRoot, 3)
	if err != nil {
		return nil, fmt.Errorf("failed to get recent commits: %w", err)
	}
	changes.RecentCommits = commits

	// Get staged changes
	stagedFiles, err := getStagedChanges(workspaceRoot, specificFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to get staged changes: %w", err)
	}
	changes.StagedFiles = stagedFiles

	// Get unstaged changes
	unstagedFiles, err := getUnstagedChanges(workspaceRoot, specificFiles)
	if err != nil {
		return nil, fmt.Errorf("failed to get unstaged changes: %w", err)
	}
	changes.UnstagedFiles = unstagedFiles

	return changes, nil
}

// generateCommitMessage uses LLM to generate an intelligent commit message
func generateCommitMessage(ctx context.Context, llmClient llm.Client, templateEngine *templates.Engine, changes *GitChanges, options CommitOptions, cfg *config.AppConfig) (string, error) {
	// Prepare the prompt with git changes analysis
	promptData := map[string]interface{}{
		"Changes":       changes,
		"Options":       options,
		"HasStaged":     len(changes.StagedFiles) > 0,
		"HasUnstaged":   len(changes.UnstagedFiles) > 0,
		"TotalFiles":    len(changes.StagedFiles) + len(changes.UnstagedFiles),
		"CurrentBranch": changes.CurrentBranch,
		"RecentCommits": changes.RecentCommits,
	}

	// Try to load commit message template
	var prompt string
	if templateEngine != nil {
		renderedPrompt, err := templateEngine.Render("commit_message.tmpl", promptData)
		if err == nil {
			prompt = renderedPrompt
		}
	}

	// Fallback to built-in prompt if template not found
	if prompt == "" {
		prompt = buildDefaultCommitPrompt(changes, options)
	}

	// Generate commit message using LLM
	response, err := llmClient.Generate(ctx, cfg.LLM.Model, prompt, "", nil)
	if err != nil {
		return "", fmt.Errorf("failed to generate commit message: %w", err)
	}

	commitMessage := strings.TrimSpace(response)

	// Clean up the commit message (remove quotes, extra whitespace)
	commitMessage = strings.Trim(commitMessage, "\"'`")
	commitMessage = strings.TrimSpace(commitMessage)

	return commitMessage, nil
}

// buildDefaultCommitPrompt creates a default prompt when no template is available
func buildDefaultCommitPrompt(changes *GitChanges, options CommitOptions) string {
	var prompt strings.Builder

	prompt.WriteString("You are an expert developer tasked with creating a high-quality commit message.\n\n")
	prompt.WriteString("Analyze the following git changes and generate a concise, conventional commit message.\n\n")

	prompt.WriteString("## Guidelines:\n")
	prompt.WriteString("- Use conventional commit format: type(scope): description\n")
	prompt.WriteString("- Types: feat, fix, docs, style, refactor, test, chore, ci, perf\n")
	prompt.WriteString("- Keep the subject line under 50 characters\n")
	prompt.WriteString("- Use imperative mood (\"add\" not \"added\")\n")
	prompt.WriteString("- Focus on the most significant change\n")
	prompt.WriteString("- Be specific but concise\n\n")

	if options.Type != "" {
		prompt.WriteString(fmt.Sprintf("- Required type: %s\n", options.Type))
	}
	if options.Scope != "" {
		prompt.WriteString(fmt.Sprintf("- Required scope: %s\n", options.Scope))
	}
	if options.Breaking {
		prompt.WriteString("- This is a BREAKING CHANGE (add ! after type/scope)\n")
	}
	prompt.WriteString("\n")

	prompt.WriteString("## Current Repository State:\n")
	prompt.WriteString(fmt.Sprintf("- Branch: %s\n", changes.CurrentBranch))

	if len(changes.RecentCommits) > 0 {
		prompt.WriteString("- Recent commits:\n")
		for _, commit := range changes.RecentCommits {
			prompt.WriteString(fmt.Sprintf("  - %s: %s\n", commit.Hash, commit.Message))
		}
	}
	prompt.WriteString("\n")

	// Add staged changes
	if len(changes.StagedFiles) > 0 {
		prompt.WriteString("## Staged Changes:\n")
		for _, file := range changes.StagedFiles {
			prompt.WriteString(fmt.Sprintf("- %s (%s): +%d -%d\n",
				file.FilePath, file.Status, file.Additions, file.Deletions))
			if file.DiffSample != "" {
				prompt.WriteString(fmt.Sprintf("  Sample: %s\n", file.DiffSample))
			}
		}
		prompt.WriteString("\n")
	}

	// Add unstaged changes
	if len(changes.UnstagedFiles) > 0 {
		prompt.WriteString("## Unstaged Changes:\n")
		for _, file := range changes.UnstagedFiles {
			prompt.WriteString(fmt.Sprintf("- %s (%s): +%d -%d\n",
				file.FilePath, file.Status, file.Additions, file.Deletions))
			if file.DiffSample != "" {
				prompt.WriteString(fmt.Sprintf("  Sample: %s\n", file.DiffSample))
			}
		}
		prompt.WriteString("\n")
	}

	prompt.WriteString("Generate ONLY the commit message (one line), no explanation or additional text:")

	return prompt.String()
}

// Helper functions for git operations
func isGitRepo(workspaceRoot string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = workspaceRoot
	return cmd.Run() == nil
}

func getCurrentBranch(workspaceRoot string) (string, error) {
	cmd := exec.Command("git", "symbolic-ref", "--short", "HEAD")
	cmd.Dir = workspaceRoot
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func getRecentCommits(workspaceRoot string, count int) ([]Commit, error) {
	cmd := exec.Command("git", "log", "--pretty=format:%h|%an|%s|%ci", fmt.Sprintf("-%d", count))
	cmd.Dir = workspaceRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var commits []Commit
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			continue
		}
		commits = append(commits, Commit{
			Hash:    parts[0],
			Author:  parts[1],
			Message: parts[2],
			Date:    parts[3],
		})
	}

	return commits, nil
}

func getStagedChanges(workspaceRoot string, specificFiles []string) ([]FileChange, error) {
	return getChanges(workspaceRoot, "--cached", specificFiles)
}

func getUnstagedChanges(workspaceRoot string, specificFiles []string) ([]FileChange, error) {
	return getChanges(workspaceRoot, "", specificFiles)
}

func getChanges(workspaceRoot string, staged string, specificFiles []string) ([]FileChange, error) {
	args := []string{"diff", "--numstat"}
	if staged != "" {
		args = append(args, staged)
	}
	if len(specificFiles) > 0 {
		args = append(args, "--")
		args = append(args, specificFiles...)
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = workspaceRoot
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var changes []FileChange
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 3 {
			continue
		}

		additions := 0
		deletions := 0
		if parts[0] != "-" {
			fmt.Sscanf(parts[0], "%d", &additions)
		}
		if parts[1] != "-" {
			fmt.Sscanf(parts[1], "%d", &deletions)
		}

		filePath := parts[2]
		status := getFileStatus(workspaceRoot, filePath, staged != "")
		diffSample := getDiffSample(workspaceRoot, filePath, staged != "")

		changes = append(changes, FileChange{
			FilePath:   filePath,
			Status:     status,
			Additions:  additions,
			Deletions:  deletions,
			DiffSample: diffSample,
		})
	}

	return changes, nil
}

func getFileStatus(workspaceRoot, filePath string, staged bool) string {
	args := []string{"status", "--porcelain", "--", filePath}
	cmd := exec.Command("git", args...)
	cmd.Dir = workspaceRoot
	out, err := cmd.Output()
	if err != nil {
		return "?"
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		return "?"
	}

	statusLine := lines[0]
	if len(statusLine) < 2 {
		return "?"
	}

	if staged {
		return string(statusLine[0]) // Staged status (first character)
	}
	return string(statusLine[1]) // Unstaged status (second character)
}

func getDiffSample(workspaceRoot, filePath string, staged bool) string {
	args := []string{"diff"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", filePath)

	cmd := exec.Command("git", args...)
	cmd.Dir = workspaceRoot
	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Get first meaningful line of diff (skip headers)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			if len(line) > 50 {
				return line[:50] + "..."
			}
			return line
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			if len(line) > 50 {
				return line[:50] + "..."
			}
			return line
		}
	}

	return ""
}

func stageFiles(workspaceRoot string, files []FileChange) error {
	filePaths := make([]string, len(files))
	for i, file := range files {
		filePaths[i] = file.FilePath
	}

	args := append([]string{"add"}, filePaths...)
	cmd := exec.Command("git", args...)
	cmd.Dir = workspaceRoot
	return cmd.Run()
}

func createCommit(workspaceRoot, message string) (string, error) {
	cmd := exec.Command("git", "commit", "-m", message)
	cmd.Dir = workspaceRoot
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	// Extract commit hash from output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "[") && strings.Contains(line, "]") {
			parts := strings.Fields(line)
			for _, part := range parts {
				if strings.HasSuffix(part, "]") && len(part) > 1 {
					hash := strings.TrimSuffix(part, "]")
					if len(hash) >= 7 {
						return hash, nil
					}
				}
			}
		}
	}

	// Fallback: get latest commit hash
	cmd = exec.Command("git", "rev-parse", "--short", "HEAD")
	cmd.Dir = workspaceRoot
	hashOutput, err := cmd.Output()
	if err != nil {
		return "unknown", nil
	}

	return strings.TrimSpace(string(hashOutput)), nil
}

func printChangeSummary(changes *GitChanges) {
	if len(changes.StagedFiles) > 0 {
		fmt.Printf("\nStaged files:\n")
		for _, file := range changes.StagedFiles {
			fmt.Printf("  %s %s (+%d -%d)\n", file.Status, file.FilePath, file.Additions, file.Deletions)
		}
	}

	if len(changes.UnstagedFiles) > 0 {
		fmt.Printf("\nUnstaged files:\n")
		for _, file := range changes.UnstagedFiles {
			fmt.Printf("  %s %s (+%d -%d)\n", file.Status, file.FilePath, file.Additions, file.Deletions)
		}
	}
}

func confirmCommit(message string, changes *GitChanges) bool {
	fmt.Printf("\nCommit message: %s\n", message)
	printChangeSummary(changes)

	fmt.Printf("\nDo you want to proceed with this commit? (y/N): ")
	var response string
	fmt.Scanln(&response)

	return strings.ToLower(strings.TrimSpace(response)) == "y"
}

func init() {
	rootCmd.AddCommand(commitCmd)

	commitCmd.Flags().BoolVar(&commitDryRun, "dry-run", false, "Preview commit message without creating commit")
	commitCmd.Flags().BoolVar(&commitAutoCommit, "auto-commit", false, "Automatically commit after generating message")
	commitCmd.Flags().StringVar(&commitType, "type", "", "Conventional commit type (feat, fix, docs, etc.)")
	commitCmd.Flags().StringVar(&commitScope, "scope", "", "Conventional commit scope")
	commitCmd.Flags().BoolVar(&commitBreaking, "breaking", false, "Mark as breaking change")
	commitCmd.Flags().BoolVar(&commitInteractive, "interactive", false, "Interactive mode with confirmation")
	commitCmd.Flags().StringSliceVar(&commitFiles, "files", []string{}, "Specific files to include in commit message analysis")
}
