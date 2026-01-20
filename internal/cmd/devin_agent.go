// Package cmd provides the devin-agent command for gt.
package cmd

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gastown/internal/devin"
	"github.com/steveyegge/gastown/internal/style"
)

// Default timeout and poll interval for Devin sessions
const (
	defaultDevinTimeout      = 4 * time.Hour
	defaultDevinPollInterval = 30 * time.Second
)

// devinAgentOptions contains options for running the devin-agent command.
type devinAgentOptions struct {
	prompt       string
	playbookID   string
	baseURL      string
	pollInterval time.Duration
	timeout      time.Duration
	stdout       io.Writer
	noWait       bool // If true, don't wait for session completion
}

var devinAgentCmd = &cobra.Command{
	Use:   "devin-agent",
	Short: "Run a task using Devin AI agent",
	Long: `Run a task using Devin AI agent via the Devin API.

This command creates a new Devin session with the given prompt and waits
for it to complete. The session ID is output for tracking.

Requires DEVIN_API_KEY environment variable to be set.

Examples:
  gt devin-agent --prompt "Fix the failing tests in src/utils"
  gt devin-agent --prompt "Add dark mode support" --playbook my-playbook-id
  gt devin-agent --prompt "Refactor the auth module" --no-wait`,
	RunE: runDevinAgent,
}

var (
	devinPrompt     string
	devinPlaybookID string
	devinNoWait     bool
	devinTimeout    time.Duration
)

func init() {
	devinAgentCmd.Flags().StringVarP(&devinPrompt, "prompt", "p", "", "Task prompt for Devin (required)")
	devinAgentCmd.Flags().StringVar(&devinPlaybookID, "playbook", "", "Playbook ID to use for the session")
	devinAgentCmd.Flags().BoolVar(&devinNoWait, "no-wait", false, "Don't wait for session completion")
	devinAgentCmd.Flags().DurationVar(&devinTimeout, "timeout", defaultDevinTimeout, "Timeout for waiting on session completion")

	_ = devinAgentCmd.MarkFlagRequired("prompt")

	rootCmd.AddCommand(devinAgentCmd)
}

func runDevinAgent(cmd *cobra.Command, args []string) error {
	return runDevinAgentWithOptions(devinAgentOptions{
		prompt:       devinPrompt,
		playbookID:   devinPlaybookID,
		baseURL:      devin.DefaultBaseURL,
		pollInterval: defaultDevinPollInterval,
		timeout:      devinTimeout,
		stdout:       os.Stdout,
		noWait:       devinNoWait,
	})
}

func runDevinAgentWithOptions(opts devinAgentOptions) error {
	// Validate API key
	apiKey := os.Getenv("DEVIN_API_KEY")
	if apiKey == "" {
		return fmt.Errorf("DEVIN_API_KEY environment variable is not set")
	}

	// Validate prompt
	if opts.prompt == "" {
		return fmt.Errorf("prompt is required")
	}

	// Set default stdout if not provided
	stdout := opts.stdout
	if stdout == nil {
		stdout = os.Stdout
	}

	// Create Devin client
	client, err := devin.NewClient(apiKey)
	if err != nil {
		return fmt.Errorf("creating Devin client: %w", err)
	}

	// Override base URL if provided (for testing)
	if opts.baseURL != "" && opts.baseURL != devin.DefaultBaseURL {
		client = client.WithBaseURL(opts.baseURL)
	}

	// Create session request
	req := devin.CreateSessionRequest{
		Prompt: opts.prompt,
	}
	if opts.playbookID != "" {
		req.PlaybookID = opts.playbookID
	}

	// Create session
	fmt.Fprintf(stdout, "Creating Devin session...\n")
	session, err := client.CreateSession(req)
	if err != nil {
		return fmt.Errorf("creating session: %w", err)
	}

	fmt.Fprintf(stdout, "%s Session created: %s\n", style.Bold.Render("✓"), session.SessionID)
	fmt.Fprintf(stdout, "Status: %s\n", session.Status)

	// Set session ID in environment for other tools to use
	os.Setenv("DEVIN_SESSION_ID", session.SessionID)

	// If no-wait, return immediately
	if opts.noWait {
		fmt.Fprintf(stdout, "\nSession running in background. Track at: https://app.devin.ai/sessions/%s\n", session.SessionID)
		return nil
	}

	// Wait for session completion
	fmt.Fprintf(stdout, "\nWaiting for session to complete (timeout: %s)...\n", opts.timeout)

	finalSession, err := client.WaitForCompletion(session.SessionID, opts.pollInterval, opts.timeout)
	if err != nil {
		return fmt.Errorf("waiting for session: %w", err)
	}

	// Check final status
	fmt.Fprintf(stdout, "\nSession completed with status: %s\n", finalSession.Status)
	fmt.Fprintf(stdout, "Session ID: %s\n", finalSession.SessionID)

	if finalSession.PullRequest != nil {
		fmt.Fprintf(stdout, "Pull Request: %s\n", finalSession.PullRequest.URL)
	}

	// Return error if session failed
	if finalSession.Status == devin.SessionStatusFailed {
		return fmt.Errorf("session failed")
	}

	return nil
}
