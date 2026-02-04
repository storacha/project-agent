package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/storacha/project-agent/internal/github"
)

// Client handles Discord webhook interactions
type Client struct {
	webhookURL string
	botToken   string
	httpClient *http.Client
}

// NewClient creates a new Discord client
func NewClient(webhookURL string) *Client {
	return &Client{
		webhookURL: webhookURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NewBotClient creates a new Discord bot client that can send DMs
func NewBotClient(botToken string) *Client {
	return &Client{
		botToken: botToken,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// WebhookMessage represents a Discord webhook message
type WebhookMessage struct {
	Content string  `json:"content,omitempty"`
	Embeds  []Embed `json:"embeds,omitempty"`
}

// Embed represents a Discord embed
type Embed struct {
	Title       string  `json:"title,omitempty"`
	Description string  `json:"description,omitempty"`
	Color       int     `json:"color,omitempty"`
	Fields      []Field `json:"fields,omitempty"`
	Timestamp   string  `json:"timestamp,omitempty"`
}

// Field represents an embed field
type Field struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// StaleIssue represents an issue that needs attention
type StaleIssue struct {
	Issue           github.Issue
	DaysSinceUpdate int
	AssignedTo      []string // GitHub usernames
}

// SendStaleIssuesReport sends a summary of stale issues to Discord
func (c *Client) SendStaleIssuesReport(ctx context.Context, staleIssues []StaleIssue, userMappings map[string]string) error {
	if len(staleIssues) == 0 {
		// Send a "all good" message
		msg := WebhookMessage{
			Content: "✅ All issues in Sprint Backlog, In Progress, and PR Review have been updated recently!",
		}
		return c.sendWebhook(ctx, msg)
	}

	// Group issues by status
	byStatus := make(map[string]map[string][]StaleIssue)
	for _, stale := range staleIssues {
		statusMap := byStatus[stale.Issue.ProjectItem.StatusValue]
		if statusMap == nil {
			statusMap = make(map[string][]StaleIssue)
			byStatus[stale.Issue.ProjectItem.StatusValue] = statusMap
		}
		if len(stale.AssignedTo) > 0 {
			mentions := ""
			for i, githubUser := range stale.AssignedTo {
				if discordID, ok := userMappings[githubUser]; ok {
					mentions += fmt.Sprintf("<@%s>", discordID)
				} else {
					mentions += fmt.Sprintf("@%s", githubUser)
				}
				if i < len(stale.AssignedTo)-1 {
					mentions += " "
				}
			}
			statusMap[mentions] = append(statusMap[mentions], stale)
		} else {
			statusMap["Unassigned"] = append(statusMap["Unassigned"], stale)
		}
	}

	// Build embed
	// Add fields for each status
	statuses := []string{"Sprint Backlog", "In Progress", "PR Review"}
	embeds := make([]Embed, 0, len(statuses))

	for _, status := range statuses {

		byAssignee := byStatus[status]
		if len(byAssignee) == 0 {
			continue
		}

		description := fmt.Sprintf("%d issues haven't been updated in 3+ days\n\n", len(byAssignee))
		for assignee, issues := range byAssignee {
			description += fmt.Sprintf("**%s**\n\n", assignee)
			for _, stale := range issues {
				// Build issue line
				line := fmt.Sprintf("• [%s #%d](%s) %s", stale.Issue.RepositoryName, stale.Issue.Number, stale.Issue.URL, stale.Issue.Title)

				// Add days since update
				line += fmt.Sprintf(" *(%d days)*", stale.DaysSinceUpdate)

				description += line + "\n"
			}
			description += "\n"
		}
		embeds = append(embeds, Embed{
			Title:       status,
			Description: description,
			Color:       0xFF9900, // Orange
			Timestamp:   time.Now().Format("2006-01-02T15:04:05Z"),
			Fields:      nil,
		})
	}

	msg := WebhookMessage{
		Content: fmt.Sprintf("⚠️ Stale Issue Report - %d issues need attention", len(staleIssues)),
		Embeds:  embeds,
	}

	return c.sendWebhook(ctx, msg)
}

// sendWebhook sends a message to the Discord webhook
func (c *Client) sendWebhook(ctx context.Context, msg WebhookMessage) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook message: %w", err)
	}

	fmt.Println("Discord Webhook Payload:", string(payload)) // Debugging line
	req, err := http.NewRequestWithContext(ctx, "POST", c.webhookURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned non-success status: %d, Body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// UserIssues groups issues by user for weekly DM
type UserIssues struct {
	GithubUsername string
	DiscordUserID  string
	Issues         []github.Issue
}

// SendWeeklyDM sends a DM to a user with their assigned issues
func (c *Client) SendWeeklyDM(ctx context.Context, userIssues UserIssues) error {
	if c.botToken == "" {
		return fmt.Errorf("bot token not configured")
	}

	// Step 1: Create a DM channel with the user
	dmChannel, err := c.createDMChannel(ctx, userIssues.DiscordUserID)
	if err != nil {
		return fmt.Errorf("failed to create DM channel: %w", err)
	}

	// Step 2: Build the message content
	content := fmt.Sprintf("👋 Hi! Here's your weekly issue update for **%s**.\n\n", userIssues.GithubUsername)
	content += fmt.Sprintf("You have **%d** issue(s) assigned to you. Please review and update any whose status has changed:\n\n", len(userIssues.Issues))

	// Group by status
	byStatus := make(map[string][]github.Issue)
	for _, issue := range userIssues.Issues {
		status := issue.ProjectItem.StatusValue
		byStatus[status] = append(byStatus[status], issue)
	}

	// Add issues by status
	statuses := []string{"Sprint Backlog", "In Progress", "PR Review"}
	for _, status := range statuses {
		issues := byStatus[status]
		if len(issues) == 0 {
			continue
		}

		content += fmt.Sprintf("**%s (%d)**\n", status, len(issues))
		for _, issue := range issues {
			content += fmt.Sprintf("• [#%d](%s) %s\n", issue.Number, issue.URL, issue.Title)
		}
		content += "\n"
	}

	content += "Please update the status of any issues that have changed, or add a comment if you're stuck or need help. Thanks! 🙏"

	// Step 3: Send the message
	msg := map[string]interface{}{
		"content": content,
	}

	return c.sendBotMessage(ctx, dmChannel, msg)
}

// DMChannel represents a Discord DM channel
type DMChannel struct {
	ID string `json:"id"`
}

// createDMChannel creates or retrieves a DM channel with a user
func (c *Client) createDMChannel(ctx context.Context, userID string) (string, error) {
	payload := map[string]interface{}{
		"recipient_id": userID,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://discord.com/api/v10/users/@me/channels", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bot "+c.botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("API returned non-success status: %d", resp.StatusCode)
	}

	var dmChannel DMChannel
	if err := json.NewDecoder(resp.Body).Decode(&dmChannel); err != nil {
		return "", fmt.Errorf("failed to decode response: %w", err)
	}

	return dmChannel.ID, nil
}

// sendBotMessage sends a message to a channel using the bot
func (c *Client) sendBotMessage(ctx context.Context, channelID string, msg map[string]interface{}) error {
	jsonPayload, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", channelID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bot "+c.botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API returned non-success status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// SendUnassignedIssuesDM sends a DM with all unassigned issues to a specified user
func (c *Client) SendUnassignedIssuesDM(ctx context.Context, discordUserID string, issues []github.Issue) error {
	if c.botToken == "" {
		return fmt.Errorf("bot token not configured")
	}

	if len(issues) == 0 {
		// No unassigned issues - send a positive message
		dmChannel, err := c.createDMChannel(ctx, discordUserID)
		if err != nil {
			return fmt.Errorf("failed to create DM channel: %w", err)
		}

		msg := map[string]interface{}{
			"content": "✅ Great news! There are no unassigned issues in Sprint Backlog, In Progress, or PR Review.",
		}
		return c.sendBotMessage(ctx, dmChannel, msg)
	}

	// Step 1: Create a DM channel with the user
	dmChannel, err := c.createDMChannel(ctx, discordUserID)
	if err != nil {
		return fmt.Errorf("failed to create DM channel: %w", err)
	}

	// Step 2: Build the message content
	content := fmt.Sprintf("⚠️ **Unassigned Issues Report**\n\n")
	content += fmt.Sprintf("There are **%d** unassigned issue(s) in active statuses. Please review and assign them:\n\n", len(issues))

	// Group by status
	byStatus := make(map[string][]github.Issue)
	for _, issue := range issues {
		status := issue.ProjectItem.StatusValue
		byStatus[status] = append(byStatus[status], issue)
	}

	// Add issues by status
	statuses := []string{"Sprint Backlog", "In Progress", "PR Review"}
	for _, status := range statuses {
		statusIssues := byStatus[status]
		if len(statusIssues) == 0 {
			continue
		}

		content += fmt.Sprintf("**%s (%d)**\n", status, len(statusIssues))
		for _, issue := range statusIssues {
			content += fmt.Sprintf("• [#%d](%s) %s\n", issue.Number, issue.URL, issue.Title)
		}
		content += "\n"
	}

	content += "Please assign these issues to the appropriate team members. Thanks! 🙏"

	// Step 3: Send the message
	msg := map[string]interface{}{
		"content": content,
	}

	return c.sendBotMessage(ctx, dmChannel, msg)
}

// ThreadResponse represents a Discord thread creation response
type ThreadResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateStandupThread creates a new thread in the standup channel and posts the standup prompt
func (c *Client) CreateStandupThread(ctx context.Context, channelID, roleID string) error {
	if c.botToken == "" {
		return fmt.Errorf("bot token not configured")
	}

	// Step 1: Create the thread
	now := time.Now()
	threadName := fmt.Sprintf("Async Standup - %s", now.Format("Monday, January 2, 2006"))

	threadPayload := map[string]interface{}{
		"name":                threadName,
		"auto_archive_duration": 1440, // 24 hours
		"type":                11,     // Public thread
	}

	jsonPayload, err := json.Marshal(threadPayload)
	if err != nil {
		return fmt.Errorf("failed to marshal thread payload: %w", err)
	}

	url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/threads", channelID)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create thread request: %w", err)
	}

	req.Header.Set("Authorization", "Bot "+c.botToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to create thread: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("thread creation returned non-success status: %d, body: %s", resp.StatusCode, string(bodyBytes))
	}

	var threadResp ThreadResponse
	if err := json.NewDecoder(resp.Body).Decode(&threadResp); err != nil {
		return fmt.Errorf("failed to decode thread response: %w", err)
	}

	// Step 2: Post the standup message in the thread
	roleMention := ""
	if roleID != "" {
		roleMention = fmt.Sprintf("<@&%s> ", roleID)
	}

	content := fmt.Sprintf("%sGood morning! 🌅\n\n", roleMention)
	content += "**It's time for async standup!** Please share:\n\n"
	content += "1️⃣ What did you work on recently?\n"
	content += "2️⃣ What are you working on today?\n"
	content += "3️⃣ Any blockers or help needed?\n\n"
	content += "Reply to this thread with your update. Thanks! 🙏"

	msg := map[string]interface{}{
		"content": content,
	}

	return c.sendBotMessage(ctx, threadResp.ID, msg)
}
