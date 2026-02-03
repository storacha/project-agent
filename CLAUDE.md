# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Project Agent is an automated GitHub backlog maintenance agent built in Go that performs maintenance tasks on GitHub Projects using GraphQL API, Gemini AI for semantic analysis, and Discord for notifications.

The system uses a **modular command architecture** where each maintenance task (stale triage, duplicate detection, PR linking, etc.) is a separate executable binary. All commands share common client libraries (GitHub, Discord, Gemini) and a centralized configuration system.

## Development Commands

### Building
```bash
# Build all commands
go build ./...

# Build specific command
go build -o bin/triage-stale cmd/triage-stale/main.go
go build -o bin/detect-duplicates cmd/detect-duplicates/main.go
```

### Testing
```bash
go test ./...
```

### Running Commands Locally
All commands use `.env` files (via godotenv) and environment variables for configuration:

```bash
# Create a .env file with required variables:
# GITHUB_TOKEN, GITHUB_ORG, PROJECT_NUMBER, GEMINI_API_KEY

# Run in dry-run mode (no mutations)
DRY_RUN=true go run cmd/triage-stale/main.go
DRY_RUN=true go run cmd/detect-duplicates/main.go
DRY_RUN=true go run cmd/link-pr/main.go
DRY_RUN=true go run cmd/scan-open-prs/main.go
DRY_RUN=true go run cmd/check-daily-updates/main.go
DRY_RUN=true go run cmd/send-weekly-dms/main.go
DRY_RUN=true go run cmd/deploy-pr-workflow/main.go

# Run for real (omit DRY_RUN or set to false)
go run cmd/triage-stale/main.go
```

## Architecture

### Command Structure Pattern

Every command follows a consistent initialization pattern:

1. Load `.env` file (if exists) via `godotenv.Load()`
2. Load config from environment via `config.LoadFromEnv()`
3. Initialize required clients (GitHub, Discord, Gemini)
4. Fetch issues from GitHub Project
5. Execute task function from `internal/tasks/`
6. Print formatted summary report

**Example from any cmd/*/main.go:**
```go
func main() {
    ctx := context.Background()
    cfg, err := config.LoadFromEnv()  // Automatically loads .env

    githubClient, err := github.NewClient(cfg.GithubToken, cfg.GithubOrg, cfg.ProjectNumber)
    similarityClient, err := similarity.NewClient(cfg.GeminiAPIKey)
    defer similarityClient.Close()

    issues, err := githubClient.GetBacklogIssues(ctx)
    report, err := tasks.DetectDuplicates(ctx, githubClient, similarityClient, issues, cfg)

    // Print report...
}
```

### Key Architectural Components

#### 1. Configuration System (internal/config/config.go)
- **Single source of truth** for all environment variables
- Automatically loads `.env` files via `godotenv.Load()` in `LoadFromEnv()`
- Provides defaults for optional values (staleness threshold: 180 days, similarity: 0.85, etc.)
- All commands use `config.LoadFromEnv()` except `deploy-pr-workflow` which loads env vars directly

#### 2. GitHub Client (internal/github/client.go)
- Uses **GraphQL API** (githubv4) exclusively for efficiency
- Auto-fetches project metadata (project ID, status field ID) on initialization
- Main methods:
  - `GetBacklogIssues(ctx)` - Fetches issues with target statuses from project
  - `GetIssueByNumber(ctx, owner, repo, number)` - Fetches specific issue if in project
  - `MoveToPRReview(ctx, issue)` - Moves issue to "PR Review" status
  - `MoveToStuckDead(ctx, issue, reason)` - Moves issue to "Stuck / Dead Issue" status
  - `AddComment(ctx, issue, body)` - Adds comment to issue
  - `AddLabel(ctx, issue, label)` - Adds label to issue (creates if needed)
  - `GetAllOpenPRs(ctx, org)` - Fetches all open PRs across organization repos

**Important:** The client automatically fetches the project's Status field metadata on initialization. All status changes use the field ID and status option IDs fetched during init.

#### 3. Similarity Client (internal/similarity/client.go)
- Wraps Google Gemini AI API for semantic similarity detection
- `CompareSimilarity(ctx, issue1, issue2)` returns similarity score (0.0-1.0)
- `FindBestMatch(ctx, prTitle, prBody, issues)` finds most similar issue to a PR
- Truncates issue bodies to 500 chars for efficiency
- **Must call `Close()` when done** to clean up resources

#### 4. Discord Client (internal/discord/client.go)
- Two client types:
  - **Webhook client** (`NewClient(webhookURL)`) - For channel notifications (daily updates)
  - **Bot client** (`NewBotClient(botToken)`) - For sending DMs (weekly updates)
- Formats rich embedded messages with status groupings and @mentions

#### 5. Issue Reference Parser (internal/parser/issue_refs.go)
- Extracts issue references from PR titles and bodies
- Supports multiple formats:
  - Simple: `#123`
  - Keywords: `fixes #123`, `closes #456`, `resolves #789`
  - Cross-repo: `storacha/guppy#123`
  - URLs: `https://github.com/storacha/guppy/issues/123`
- Returns structured references with owner, repo, number, and whether it's explicit (keyword-based)

### Task Execution Flow

All tasks follow this pattern:

1. **Fetch data** from GitHub (issues, PRs, etc.)
2. **Process data** with business logic (detect duplicates, check staleness, parse references)
3. **Perform mutations** (move issues, add comments/labels) with dry-run support
4. **Build report** with metrics and errors
5. **Return report** to command for printing

**Dry-run mode:** All mutation operations check `cfg.DryRun` and log intended actions without making changes.

### PR Linking Workflow (Distributed Architecture)

PR linking uses a **distributed workflow approach** to avoid storing secrets in every repository:

1. **Repository workflows** (`.github/workflows/notify-pr.yml` in each repo):
   - Triggered on PR open/edit
   - Send `repository_dispatch` event to `project-agent` repo with PR details

2. **Central receiver** (`.github/workflows/handle-pr-link.yml` in project-agent):
   - Receives dispatch events
   - Has access to all secrets (GitHub token, Gemini API key)
   - Runs `link-pr` command with PR details

3. **Link-PR logic** (internal/tasks/pr_linking.go):
   - First tries **direct reference matching** (parse PR for issue refs)
   - Falls back to **semantic matching** (use Gemini to find best match among "In Progress"/"Sprint Backlog" issues)
   - Direct matches: moves all referenced issues to "PR Review" (GitHub auto-creates links)
   - Semantic match: moves best match to "PR Review" and adds minimal comment (creates cross-reference)

**Deployment:** Use `cmd/deploy-pr-workflow/main.go` to deploy notify workflows to all org repos.

## Project Status Field Requirements

The agent expects these Status field values in your GitHub Project:
- **Inbox** - New issues
- **Backlog** - Issues to be worked on
- **Sprint Backlog** - Issues planned for current sprint
- **In Progress** - Issues actively being worked on
- **PR Review** - Issues with associated PRs under review
- **Stuck / Dead Issue** - Where stale issues are moved

You can customize target statuses via `TARGET_STATUSES` env var (comma-separated).

## Adding New Commands

Follow the existing pattern in `cmd/*/main.go`:

1. Create task function in `internal/tasks/` that returns a report struct
2. Create command in `cmd/my-task/main.go` following the standard pattern
3. Create GitHub Actions workflow in `.github/workflows/my-task.yml`
4. Task should accept `ctx`, relevant clients, issues, and `cfg` as parameters
5. Task should check `cfg.DryRun` before any mutations

## Environment Variables

All configuration is in `internal/config/config.go`. Key variables:

**Required:**
- `GITHUB_TOKEN` - GitHub PAT with repo and project scopes
- `GITHUB_ORG` - Organization name
- `PROJECT_NUMBER` - GitHub Project number (visible in project URL)
- `GEMINI_API_KEY` - Google Gemini API key

**Optional:**
- `DRY_RUN` - Set to "true" to prevent mutations
- `STALENESS_THRESHOLD_DAYS` - Default: 180
- `DUPLICATE_SIMILARITY` - Default: 0.85 (85% similarity)
- `DAILY_UPDATE_THRESHOLD` - Default: 3 days
- `TARGET_STATUSES` - Default: "Inbox, Backlog, Sprint Backlog, In Progress, PR Review"
- `DISCORD_WEBHOOK_URL` - For channel notifications
- `DISCORD_BOT_TOKEN` - For DM sending
- `USER_MAPPINGS` - JSON mapping GitHub usernames to Discord user IDs

## Important Implementation Details

### GraphQL Pagination
The GitHub client handles pagination automatically. When fetching issues or PRs, it uses cursor-based pagination with `first: 100` and continues until `hasNextPage` is false.

### Rate Limiting
Commands include 2-second delays between mutation operations to respect GitHub API rate limits. See individual task implementations.

### Error Handling
Tasks collect errors in report structs rather than failing immediately. This allows partial success - some issues can be processed even if others fail.

### Issue Body Truncation
Similarity detection truncates issue bodies to 500 characters to stay within Gemini API token limits and improve performance.

### Label Creation
`AddLabel()` automatically creates labels if they don't exist in the repository. The "possible duplicate" label is created with color `#d4c5f9` (light purple).

## GitHub Actions Integration

All workflows follow this pattern:
- Use `actions/checkout@v4` and `actions/setup-go@v5`
- Set Go version to '1.22'
- Run command via `go run cmd/*/main.go`
- Use secrets from repository settings (prefixed with `secrets.`)
- Support `workflow_dispatch` for manual triggering

Schedule examples:
- Daily: `cron: '0 9 * * *'`
- Weekly: `cron: '0 10 * * 1'` (Mondays)
