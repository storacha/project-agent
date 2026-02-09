package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/shurcooL/githubv4"
	"golang.org/x/crypto/nacl/box"
	"golang.org/x/oauth2"
)

const workflowContent = `name: Notify PR Event

on:
  pull_request:
    types: [opened, edited]

jobs:
  notify:
    runs-on: ubuntu-latest
    steps:
      - name: Send repository_dispatch to project-agent
        run: |
          curl -X POST \
            -H "Accept: application/vnd.github.v3+json" \
            -H "Authorization: token ${{ secrets.PROJECT_AGENT_PAT }}" \
            https://api.github.com/repos/storacha/project-agent/dispatches \
            -d "{\"event_type\":\"pr-event\",\"client_payload\":{\"pr_repo\":\"${{ github.repository }}\",\"pr_number\":${{ github.event.pull_request.number }},\"pr_author\":\"${{ github.event.pull_request.user.login }}\",\"pr_title\":$(echo '${{ github.event.pull_request.title }}' | jq -Rs .),\"pr_body\":$(echo '${{ github.event.pull_request.body }}' | jq -Rs .)}}"
`

type Repository struct {
	Name          string
	DefaultBranch struct {
		Name string
	}
}

func main() {
	// Load .env file if it exists (ignore error if file doesn't exist)
	_ = godotenv.Load()

	ctx := context.Background()

	githubToken := os.Getenv("GITHUB_TOKEN")
	if githubToken == "" {
		log.Fatal("GITHUB_TOKEN environment variable is required")
	}

	projectAgentPAT := os.Getenv("PROJECT_AGENT_PAT")
	if projectAgentPAT == "" {
		log.Fatal("PROJECT_AGENT_PAT environment variable is required (PAT for repository_dispatch events)")
	}

	org := os.Getenv("GITHUB_ORG")
	if org == "" {
		org = "storacha"
	}

	dryRun := os.Getenv("DRY_RUN") == "true"

	// Create GraphQL client
	src := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: githubToken},
	)
	httpClient := oauth2.NewClient(ctx, src)
	client := githubv4.NewClient(httpClient)

	log.Printf("Fetching repositories for organization: %s\n", org)

	// Fetch all repositories
	var query struct {
		Organization struct {
			Repositories struct {
				Nodes    []Repository
				PageInfo struct {
					EndCursor   githubv4.String
					HasNextPage bool
				}
			} `graphql:"repositories(first: 100, after: $cursor)"`
		} `graphql:"organization(login: $org)"`
	}

	variables := map[string]interface{}{
		"org":    githubv4.String(org),
		"cursor": (*githubv4.String)(nil),
	}

	var allRepos []Repository

	for {
		if err := client.Query(ctx, &query, variables); err != nil {
			log.Fatalf("Failed to query repositories: %v", err)
		}

		allRepos = append(allRepos, query.Organization.Repositories.Nodes...)

		if !query.Organization.Repositories.PageInfo.HasNextPage {
			break
		}

		variables["cursor"] = githubv4.NewString(query.Organization.Repositories.PageInfo.EndCursor)
	}

	log.Printf("Found %d repositories\n", len(allRepos))

	// Deploy workflow to each repository
	deploymentCount := 0
	skippedCount := 0
	errorCount := 0

	for _, repo := range allRepos {
		// Skip project-agent itself
		if repo.Name == "project-agent" {
			log.Printf("Skipping project-agent repository\n")
			skippedCount++
			continue
		}

		log.Printf("\nProcessing: %s/%s\n", org, repo.Name)

		// Check if workflow already exists
		workflowPath := ".github/workflows/notify-pr.yml"
		exists, err := checkFileExists(ctx, client, org, repo.Name, workflowPath, repo.DefaultBranch.Name)
		if err != nil {
			log.Printf("  ERROR: Failed to check if workflow exists: %v\n", err)
			errorCount++
			continue
		}

		if exists {
			log.Printf("  Workflow already exists, skipping\n")
			skippedCount++
			continue
		}

		if dryRun {
			log.Printf("  [DRY RUN] Would set PROJECT_AGENT_PAT secret\n")
			log.Printf("  [DRY RUN] Would create workflow at %s\n", workflowPath)
			deploymentCount++
		} else {
			// Set the PROJECT_AGENT_PAT secret first
			log.Printf("  Setting PROJECT_AGENT_PAT secret...\n")
			if err := setRepositorySecret(ctx, githubToken, org, repo.Name, "PROJECT_AGENT_PAT", projectAgentPAT); err != nil {
				log.Printf("  ERROR: Failed to set secret: %v\n", err)
				errorCount++
				continue
			}
			log.Printf("  Successfully set secret\n")

			// Then create the workflow file
			if err := createWorkflowFile(ctx, githubToken, org, repo.Name, repo.DefaultBranch.Name, workflowPath); err != nil {
				log.Printf("  ERROR: Failed to create workflow: %v\n", err)
				errorCount++
			} else {
				log.Printf("  Successfully created workflow\n")
				deploymentCount++
			}
			time.Sleep(2 * time.Second) // Rate limiting
		}
	}

	// Print summary
	fmt.Println("\n" + "==========================================================")
	fmt.Println("DEPLOYMENT SUMMARY")
	fmt.Println("==========================================================")
	fmt.Printf("Total repositories: %d\n", len(allRepos))
	fmt.Printf("Workflows deployed: %d\n", deploymentCount)
	fmt.Printf("Skipped: %d\n", skippedCount)
	fmt.Printf("Errors: %d\n", errorCount)
	fmt.Println("==========================================================")

	if dryRun {
		fmt.Println("\nThis was a dry run. Set DRY_RUN=false to perform actual deployment.")
	}
}

func checkFileExists(ctx context.Context, client *githubv4.Client, owner, repo, path, branch string) (bool, error) {
	var query struct {
		Repository struct {
			Object struct {
				Blob struct {
					Text string
				} `graphql:"... on Blob"`
			} `graphql:"object(expression: $expression)"`
		} `graphql:"repository(owner: $owner, name: $name)"`
	}

	variables := map[string]interface{}{
		"owner":      githubv4.String(owner),
		"name":       githubv4.String(repo),
		"expression": githubv4.String(fmt.Sprintf("%s:%s", branch, path)),
	}

	err := client.Query(ctx, &query, variables)
	if err != nil {
		// File doesn't exist or other error
		return false, nil
	}

	return true, nil
}

func createWorkflowFile(ctx context.Context, token, owner, repo, branch, path string) error {
	// Use REST API for file creation
	// This is simpler than using GraphQL mutations for file operations

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, path)

	content := base64.StdEncoding.EncodeToString([]byte(workflowContent))

	payload := map[string]interface{}{
		"message": "Add PR notification workflow for project-agent",
		"content": content,
		"branch":  branch,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

type PublicKey struct {
	KeyID string `json:"key_id"`
	Key   string `json:"key"`
}

func setRepositorySecret(ctx context.Context, token, owner, repo, secretName, secretValue string) error {
	// Step 1: Get the repository's public key
	publicKey, err := getRepositoryPublicKey(ctx, token, owner, repo)
	if err != nil {
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Step 2: Encrypt the secret value
	encryptedValue, err := encryptSecret(secretValue, publicKey.Key)
	if err != nil {
		return fmt.Errorf("failed to encrypt secret: %w", err)
	}

	// Step 3: Set the encrypted secret
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/secrets/%s", owner, repo, secretName)

	payload := map[string]interface{}{
		"encrypted_value": encryptedValue,
		"key_id":          publicKey.KeyID,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "PUT", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

func getRepositoryPublicKey(ctx context.Context, token, owner, repo string) (*PublicKey, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/actions/secrets/public-key", owner, repo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var publicKey PublicKey
	if err := json.NewDecoder(resp.Body).Decode(&publicKey); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &publicKey, nil
}

func encryptSecret(secretValue, publicKeyStr string) (string, error) {
	// Decode the public key from base64
	publicKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyStr)
	if err != nil {
		return "", fmt.Errorf("failed to decode public key: %w", err)
	}

	// Convert to [32]byte for nacl/box
	var publicKey [32]byte
	copy(publicKey[:], publicKeyBytes)

	// Encrypt using nacl/box (anonymous encryption)
	// Generate ephemeral key pair
	ephemeralPublic, ephemeralPrivate, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Encrypt the secret
	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	encrypted := box.Seal(nonce[:], []byte(secretValue), &nonce, &publicKey, ephemeralPrivate)

	// The result should be: ephemeral public key + encrypted data
	result := append(ephemeralPublic[:], encrypted...)

	// Encode to base64
	return base64.StdEncoding.EncodeToString(result), nil
}
