package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config holds all configuration for the agent
type Config struct {
	// GitHub configuration
	GithubToken   string
	GithubOrg     string
	ProjectNumber int

	// Gemini AI configuration
	GeminiAPIKey string

	// Discord configuration
	DiscordWebhookURL       string
	DiscordBotToken         string
	DiscordStandupChannelID string            // Discord channel ID for async standup threads
	DiscordStandupRoleID    string            // Discord role ID to mention in standup threads
	UserMappings            map[string]string // GitHub username -> Discord user ID
	UnassignedIssuesUserID  string            // Discord user ID to receive unassigned issues
	DailyUpdateThreshold    int               // Days since last update to flag as stale

	// Agent behavior configuration
	StalenessThresholdDays int
	DuplicateSimilarity    float64
	DryRun                 bool
	TargetStatuses         []string // Which statuses to analyze
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() (*Config, error) {
	// Load .env file if it exists (ignore error if file doesn't exist)
	_ = godotenv.Load()

	cfg := &Config{
		// Defaults
		StalenessThresholdDays: 180, // 6 months
		DuplicateSimilarity:    0.85, // 85% similarity threshold
		DailyUpdateThreshold:   3,    // 3 days
		DryRun:                 false,
		TargetStatuses:         []string{"Inbox", "Backlog", "Sprint Backlog", "In Progress", "PR Review"},
		UserMappings:           make(map[string]string),
	}

	// Required fields
	cfg.GithubToken = os.Getenv("GITHUB_TOKEN")
	if cfg.GithubToken == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN environment variable is required")
	}

	cfg.GithubOrg = os.Getenv("GITHUB_ORG")
	if cfg.GithubOrg == "" {
		return nil, fmt.Errorf("GITHUB_ORG environment variable is required")
	}

	projectNumStr := os.Getenv("PROJECT_NUMBER")
	if projectNumStr == "" {
		return nil, fmt.Errorf("PROJECT_NUMBER environment variable is required")
	}
	projectNum, err := strconv.Atoi(projectNumStr)
	if err != nil {
		return nil, fmt.Errorf("PROJECT_NUMBER must be a valid integer: %w", err)
	}
	cfg.ProjectNumber = projectNum

	// Gemini AI is optional - only needed for similarity detection tasks
	cfg.GeminiAPIKey = os.Getenv("GEMINI_API_KEY")

	// Optional overrides
	if thresholdStr := os.Getenv("STALENESS_THRESHOLD_DAYS"); thresholdStr != "" {
		threshold, err := strconv.Atoi(thresholdStr)
		if err != nil {
			return nil, fmt.Errorf("STALENESS_THRESHOLD_DAYS must be a valid integer: %w", err)
		}
		cfg.StalenessThresholdDays = threshold
	}

	if simStr := os.Getenv("DUPLICATE_SIMILARITY"); simStr != "" {
		sim, err := strconv.ParseFloat(simStr, 64)
		if err != nil {
			return nil, fmt.Errorf("DUPLICATE_SIMILARITY must be a valid float: %w", err)
		}
		cfg.DuplicateSimilarity = sim
	}

	if dryRunStr := os.Getenv("DRY_RUN"); dryRunStr == "true" {
		cfg.DryRun = true
	}

	if statusesStr := os.Getenv("TARGET_STATUSES"); statusesStr != "" {
		// Split by comma and trim spaces
		statuses := []string{}
		for _, s := range splitAndTrim(statusesStr, ",") {
			if s != "" {
				statuses = append(statuses, s)
			}
		}
		if len(statuses) > 0 {
			cfg.TargetStatuses = statuses
		}
	}

	// Discord configuration (optional for most commands)
	cfg.DiscordWebhookURL = os.Getenv("DISCORD_WEBHOOK_URL")
	cfg.DiscordBotToken = os.Getenv("DISCORD_BOT_TOKEN")
	cfg.DiscordStandupChannelID = os.Getenv("DISCORD_STANDUP_CHANNEL_ID")
	cfg.DiscordStandupRoleID = os.Getenv("DISCORD_STANDUP_ROLE_ID")
	cfg.UnassignedIssuesUserID = os.Getenv("UNASSIGNED_ISSUES_USER_ID")

	if updateThresholdStr := os.Getenv("DAILY_UPDATE_THRESHOLD"); updateThresholdStr != "" {
		threshold, err := strconv.Atoi(updateThresholdStr)
		if err != nil {
			return nil, fmt.Errorf("DAILY_UPDATE_THRESHOLD must be a valid integer: %w", err)
		}
		cfg.DailyUpdateThreshold = threshold
	}

	// Load GitHub -> Discord user mappings from JSON
	if mappingsJSON := os.Getenv("USER_MAPPINGS"); mappingsJSON != "" {
		if err := json.Unmarshal([]byte(mappingsJSON), &cfg.UserMappings); err != nil {
			return nil, fmt.Errorf("USER_MAPPINGS must be valid JSON: %w", err)
		}
	}

	return cfg, nil
}

// splitAndTrim splits a string by delimiter and trims whitespace from each part
func splitAndTrim(s, delim string) []string {
	parts := []string{}
	for _, part := range splitString(s, delim) {
		trimmed := trimSpace(part)
		if trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	return parts
}

func splitString(s, delim string) []string {
	if s == "" {
		return []string{}
	}
	var parts []string
	var current string
	for _, char := range s {
		if string(char) == delim {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(char)
		}
	}
	parts = append(parts, current)
	return parts
}

func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
