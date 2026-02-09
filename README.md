# Project Agent

An automated GitHub backlog maintenance agent that helps keep your GitHub Projects organized by:

1. **Identifying stale issues** - Automatically moves issues that haven't been updated in 6+ months to "Stuck / Dead Issue" status
2. **Detecting duplicates** - Uses AI (Gemini) to find semantically similar issues that may be duplicates
3. **Linking PRs to issues** - Automatically detects issue references in PRs and moves issues to PR Review status
4. **Daily update reminders** - Sends Discord notifications for active issues not updated in 3+ days
5. **Processing Initiatives** - Automatically adds sub-issues from Initiative-type issues to the project and tags them
6. **Async standup threads** - Creates Discord threads for team standup updates (Tue/Wed/Thu)
7. **Adding helpful comments** - Explains why actions were taken and provides guidance for maintainers

## Features

- ✅ **Modular Commands** - Separate commands for different maintenance tasks
- ✅ **Stale Issue Triage** - Identifies and moves stale issues (runs daily)
- ✅ **Duplicate Detection** - Uses Gemini AI to find similar issues (runs weekly)
- ✅ **PR-to-Issue Linking** - Automatically links PRs to issues and moves them to PR Review (runs on PR open/edit)
- ✅ **Daily Update Checks** - Discord notifications for active issues needing attention (runs daily)
- ✅ **Initiative Processing** - Automatically adds sub-issues from Initiatives to the project (runs daily)
- ✅ **Async Standup Threads** - Creates Discord threads for team standup updates (Tue/Wed/Thu)
- ✅ **Flexible Scheduling** - Different cron schedules for different tasks
- ✅ **Discord Integration** - Rich embedded messages with @mentions
- ✅ **Configurable** - Adjust thresholds and behavior via environment variables
- ✅ **Dry-run mode** - Test safely without making changes
- ✅ **GitHub Actions Ready** - Automated workflows included

## Setup

### Prerequisites

1. A GitHub account with access to the target organization and project
2. A Google Gemini API key ([get one here](https://ai.google.dev/))
3. Go 1.22+ (for local development)

### Installation

1. **Fork or clone this repository** to your organization

2. **Create a GitHub Personal Access Token** with the following permissions:
   - `repo` (full control)
   - `project` (read/write)
   - `write:org` (for organization projects)

3. **Set up Discord integration**:

   **For channel notifications (daily updates):**
   - Go to your Discord server settings → Integrations → Webhooks
   - Create a new webhook for the channel where you want notifications
   - Copy the webhook URL

   **For DMs (weekly updates):**
   - Go to [Discord Developer Portal](https://discord.com/developers/applications)
   - Create a new application
   - Go to "Bot" section and create a bot
   - Copy the bot token
   - Enable "Message Content Intent" under Privileged Gateway Intents
   - Invite bot to your server with "Send Messages" permission
   - Bot URL: `https://discord.com/api/oauth2/authorize?client_id=YOUR_CLIENT_ID&permissions=2048&scope=bot`

4. **Add GitHub Secrets** to your repository:
   - `PROJECT_MAINTENANCE_TOKEN` - Your GitHub PAT from step 2
   - `GEMINI_API_KEY` - Your Google Gemini API key
   - `DISCORD_WEBHOOK_URL` - Your Discord webhook URL (for channel notifications)
   - `DISCORD_BOT_TOKEN` - Your Discord bot token (for DMs)
   - `USER_MAPPINGS` - JSON mapping of GitHub usernames to Discord user IDs (see below)
   - `UNASSIGNED_ISSUES_USER_ID` - Discord user ID to receive unassigned issues report (optional)

### User Mappings Format

The `USER_MAPPINGS` secret should be a JSON object mapping GitHub usernames to Discord user IDs:

```json
{
  "github-username": "123456789012345678",
  "another-user": "987654321098765432"
}
```

To get a Discord user ID:
1. Enable Developer Mode in Discord (Settings → Advanced → Developer Mode)
2. Right-click on a user and select "Copy User ID"

5. **Configure the workflows** by editing the files in `.github/workflows/`:

   **For stale issue triage** (`.github/workflows/triage-stale.yml`):
   ```yaml
   schedule:
     - cron: '0 9 * * *'  # Daily at 9 AM UTC
   env:
     GITHUB_ORG: your-org-name
     PROJECT_NUMBER: your-project-number
     STALENESS_THRESHOLD_DAYS: 180  # Adjust as needed
   ```

   **For duplicate detection** (`.github/workflows/detect-duplicates.yml`):
   ```yaml
   schedule:
     - cron: '0 10 * * 1'  # Weekly on Mondays at 10 AM UTC
   env:
     GITHUB_ORG: your-org-name
     PROJECT_NUMBER: your-project-number
     DUPLICATE_SIMILARITY: 0.85  # 0.0-1.0, higher = stricter
   ```

### Running Locally

```bash
# Install dependencies
go mod download

# Set environment variables
export GITHUB_TOKEN="your-github-token"
export GITHUB_ORG="storacha"
export PROJECT_NUMBER="1"
export GEMINI_API_KEY="your-gemini-key"

# Run stale issue triage (in dry-run mode)
export DRY_RUN="true"
go run cmd/triage-stale/main.go

# Run duplicate detection (in dry-run mode)
go run cmd/detect-duplicates/main.go

# Deploy PR notification workflows to all repos (in dry-run mode)
go run cmd/deploy-pr-workflow/main.go

# Scan and process all existing open PRs (in dry-run mode)
go run cmd/scan-open-prs/main.go

# Run process-initiatives (in dry-run mode)
go run cmd/process-initiatives/main.go

# Run async-standup (in dry-run mode)
go run cmd/async-standup/main.go

# Run for real (remove DRY_RUN)
unset DRY_RUN
go run cmd/triage-stale/main.go
```

## Configuration

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `GITHUB_TOKEN` | Yes | - | GitHub Personal Access Token |
| `GITHUB_ORG` | Yes | - | GitHub organization name |
| `PROJECT_NUMBER` | Yes | - | GitHub Project number |
| `GEMINI_API_KEY` | Yes | - | Google Gemini API key |
| `STALENESS_THRESHOLD_DAYS` | No | 180 | Days of inactivity before marking as stale |
| `DUPLICATE_SIMILARITY` | No | 0.85 | Similarity threshold (0.0-1.0) for duplicates |
| `DAILY_UPDATE_THRESHOLD` | No | 3 | Days since last update to flag for daily check |
| `DISCORD_WEBHOOK_URL` | No | - | Discord webhook URL for channel notifications |
| `DISCORD_BOT_TOKEN` | No | - | Discord bot token for sending DMs |
| `DISCORD_STANDUP_CHANNEL_ID` | No | - | Discord channel ID for async standup threads |
| `DISCORD_STANDUP_ROLE_ID` | No | - | Discord role ID to mention in standup threads |
| `USER_MAPPINGS` | No | {} | JSON mapping of GitHub usernames to Discord IDs |
| `UNASSIGNED_ISSUES_USER_ID` | No | - | Discord user ID to receive unassigned issues report |
| `TARGET_STATUSES` | No | "Inbox, Backlog, Sprint Backlog, In Progress, PR Review" | Comma-separated list of statuses to analyze |
| `DRY_RUN` | No | false | If "true", no changes are made |

## How It Works

### 1. Stale Issue Detection

The agent:
1. Fetches all issues with target statuses (Inbox, Backlog, Sprint Backlog, In Progress, PR Review) from the project
2. Checks each issue's `updated_at` timestamp
3. If not updated in `STALENESS_THRESHOLD_DAYS`, the issue is marked as stale
4. Adds a comment explaining the situation
5. Moves the issue to "Stuck / Dead Issue" status

**Example Comment:**
```
This issue has been automatically moved to **Stuck / Dead Issue** status.

**Reason:** No activity for 213 days (threshold: 180 days)

If this issue is still relevant and you'd like to work on it, please:
1. Comment on this issue with an update
2. Move it back to Backlog or another appropriate status
3. Consider if this should be moved to Icebox instead

---
*Automated by project-agent*
```

### 2. Duplicate Detection

The agent:
1. Compares all issues with target statuses pairwise using Gemini AI
2. Looks for semantic similarity in:
   - Issue titles
   - Issue descriptions
   - Technical concepts
   - User goals
3. Groups issues with similarity ≥ `DUPLICATE_SIMILARITY`
4. Adds a `possible duplicate` label to all issues in each duplicate group

**Label Details:**
- **Label name**: `possible duplicate`
- **Color**: Light purple (`#d4c5f9`)
- **Auto-created**: If the label doesn't exist in a repository, it will be created automatically
- **Visibility**: Labels appear in the GitHub Project view, making duplicates easy to spot

The label allows you to:
- Filter issues by `label:"possible duplicate"` in your project
- See duplicate candidates at a glance in the project board
- Manually review and close/link related issues
- Remove the label if it's a false positive

### 3. PR-to-Issue Linking

When a PR is opened or edited in any repository in your organization, the agent first checks if the PR author is a team member (present in USER_MAPPINGS). External contributor PRs are skipped. For team member PRs, the agent:
1. **Parses direct issue references** from PR title and body:
   - Simple references: `#123`
   - Keyword references: `fixes #123`, `closes #456`, `resolves #789`
   - Cross-repo references: `storacha/guppy#123`
   - URL references: `https://github.com/storacha/guppy/issues/123`
2. **Checks if referenced issues are in the project**
3. **If no direct references found**, performs semantic matching:
   - Compares PR against issues with "In Progress" or "Sprint Backlog" status
   - Uses Gemini AI to find the best semantic match
   - Only matches if similarity ≥ 0.95 (stricter than duplicate detection)
4. **Takes action**:
   - **Direct references**: Moves all referenced issues to "PR Review" status
     - GitHub automatically creates the link/cross-reference
   - **Semantic match**: Moves the best matching issue to "PR Review" status
     - Adds a minimal comment to create the cross-reference link

**How it works across repos:**

The agent uses a distributed workflow approach:
1. Each repository in your organization has a lightweight workflow (`.github/workflows/notify-pr.yml`)
2. When a PR is opened/edited, the workflow uses the `PROJECT_AGENT_PAT` secret to send a `repository_dispatch` event to the `project-agent` repository
3. The `project-agent` repository receives the event and runs the linking logic with all necessary secrets (GitHub token, Gemini API key)
4. This approach minimizes secret distribution - only the dispatch PAT is stored in each repo, while sensitive secrets remain centralized

**Deploying to your repositories:**

Use the included deployment tool to add the workflow to all your repos:

```bash
# Dry run (preview what would be deployed)
export GITHUB_TOKEN="your-admin-token"        # PAT with admin:org and repo scopes
export PROJECT_AGENT_PAT="your-dispatch-token" # PAT with repo scope for repository_dispatch
export GITHUB_ORG="storacha"
export DRY_RUN="true"
go run cmd/deploy-pr-workflow/main.go

# Deploy for real
unset DRY_RUN
go run cmd/deploy-pr-workflow/main.go
```

The deployment tool will:
- Find all repositories in your organization
- Skip repositories that already have the workflow
- Skip the `project-agent` repository itself
- Set the `PROJECT_AGENT_PAT` secret in each repository (for cross-repo dispatch events)
- Create `.github/workflows/notify-pr.yml` in each repository

**Processing existing open PRs:**

After deploying the workflows, you'll want to process all currently open PRs to link them to issues. Use the scan command:

```bash
# Dry run (see what would be processed)
export GITHUB_TOKEN="your-token"
export GITHUB_ORG="storacha"
export PROJECT_NUMBER="1"
export GEMINI_API_KEY="your-key"
export USER_MAPPINGS='{"alice":"123456789012345678","bob":"987654321098765432"}'
export DRY_RUN="true"
go run cmd/scan-open-prs/main.go

# Process for real
unset DRY_RUN
go run cmd/scan-open-prs/main.go
```

The scan command will:
- Find all open PRs across all repositories in your organization
- Skip PRs from external contributors (only processes team members in USER_MAPPINGS)
- Process each PR through the same linking logic
- Link PRs to issues and move them to PR Review status
- Provide a detailed summary report of all actions taken

### 4. Daily Update Checks

Every day, the agent checks for issues in active statuses that haven't been updated recently and sends a Discord notification.

The agent:
1. **Fetches active issues** with statuses: "Sprint Backlog", "In Progress", "PR Review"
2. **Checks last update time** for each issue (status changes or comments)
3. **Identifies stale issues** not updated in 3+ days (configurable)
4. **Sends Discord notification** with:
   - Rich embedded message grouped by status
   - Issue links and titles
   - Days since last update
   - @mentions for assigned team members (using GitHub → Discord mapping)

**Example Discord Message:**

```
⚠️ Stale Issue Report - 5 issues need attention
The following issues haven't been updated in 3+ days:

Sprint Backlog (2)
• #123 Fix authentication bug (5 days) @username
• #456 Update documentation (4 days)

In Progress (2)
• #789 Implement new feature (7 days) @username @another-user
• #234 Refactor code (3 days)

PR Review (1)
• #567 Add tests (6 days) @username
```

If all issues have been updated recently, it sends a positive confirmation message instead.

### 5. Weekly DMs

Every Monday, the agent sends a direct message to each team member with their assigned issues.

The agent:
1. **Fetches all active issues** with statuses: "Sprint Backlog", "In Progress", "PR Review"
2. **Groups issues by assignee** based on GitHub usernames
3. **Sends individual DMs** to each person in the user mappings with:
   - List of their assigned issues grouped by status
   - Issue titles with links
   - Reminder to update status or comment if stuck
4. **Sends unassigned issues report** to designated user (if configured):
   - All unassigned issues in active statuses
   - Grouped by status
   - Request to assign them to team members

**Example DM:**

```
👋 Hi! Here's your weekly issue update for alice.

You have 3 issue(s) assigned to you. Please review and update any whose status has changed:

In Progress (2)
• #123 Implement authentication feature
• #456 Fix database migration issue

PR Review (1)
• #789 Add unit tests for new API

Please update the status of any issues that have changed, or add a comment if you're stuck or need help. Thanks! 🙏
```

**Example Unassigned Issues DM:**

```
⚠️ Unassigned Issues Report

There are 4 unassigned issue(s) in active statuses. Please review and assign them:

Sprint Backlog (2)
• #234 Implement caching layer
• #567 Update API documentation

In Progress (2)
• #890 Fix memory leak
• #123 Add error handling

Please assign these issues to the appropriate team members. Thanks! 🙏
```

**Requirements:**
- Discord Bot (not webhook) - can send DMs
- Users must share a server with the bot
- Bot needs "Send Messages" permission
- Set `UNASSIGNED_ISSUES_USER_ID` to receive unassigned issues report

### 6. Initiative Processing

Every day, the agent processes Initiative-type issues and their sub-issues.

The agent:
1. **Fetches all Initiatives** - Queries the project for all issues with GitHub organization-level issue type = "Initiative"
2. **Recursively fetches sub-issues** - Uses GitHub's `subIssues` field to get all sub-issues and descendants
3. **Adds sub-issues to project** - Adds each sub-issue to the project with "Inbox" status (if not already in project)
4. **Updates Initiative field** - Sets the "Initiative" text field to the parent Initiative's title
5. **Updates on changes** - Re-runs daily to capture initiative title changes and newly added sub-issues

**Requirements:**
- Your organization must have GitHub issue types configured ([see docs](https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/managing-issue-types-in-an-organization))
- At least one issue type named "Initiative"
- Your project must have a text field named "Initiative"
- Initiatives must have sub-issues added via GitHub's sub-issues feature

**Example scenario:**

Initiative #330 "Warm Storage Launch" has 76 sub-issues across multiple repositories:
- storacha/project-tracking#333: PDP Enablement
- storacha/piri#39: PDP Unit Tests
- storacha/guppy#12: bring the code up to date
- ...and 73 more

The agent will:
- Add all 76 sub-issues to the project (if not already present) with status "Inbox"
- Set the "Initiative" field to "Warm Storage Launch" for all of them
- Update the Initiative field daily in case the Initiative title changes

### 7. Async Standup Threads

On Tuesday, Wednesday, and Thursday, the agent creates a new Discord thread for async standup.

The agent:
1. **Creates a new thread** in the configured "Async Standup" channel
2. **Names the thread** with the current date (e.g., "Async Standup - Tuesday, February 3, 2026")
3. **Posts a standup prompt** with:
   - Role mention (e.g., @Storacha Team) if configured
   - Prompts for: what you worked on, what you're working on today, and any blockers
4. **Auto-archives after 24 hours** to keep the channel organized

**Requirements:**
- Discord Bot (not webhook) with permissions to:
  - Create public threads
  - Send messages in threads
- Set `DISCORD_STANDUP_CHANNEL_ID` to the channel where threads should be created
- Set `DISCORD_STANDUP_ROLE_ID` to the role to mention (optional)

**Example thread message:**

```
@Storacha Team Good morning! 🌅

**It's time for async standup!** Please share:

1️⃣ What did you work on recently?
2️⃣ What are you working on today?
3️⃣ Any blockers or help needed?

Reply to this thread with your update. Thanks! 🙏
```

**How to get Discord IDs:**
- **Channel ID**: Enable Developer Mode in Discord, right-click the channel, select "Copy Channel ID"
- **Role ID**: Type `\@RoleName` in any channel and copy the numeric ID from the output

## GitHub Actions Workflows

The agent runs automatically on different schedules:

- **Stale Issue Triage**: Daily at 9 AM UTC
- **Duplicate Detection**: Weekly on Mondays at 10 AM UTC
- **Initiative Processing**: Daily at 10 AM UTC
- **Daily Update Checks**: Daily at 2 PM UTC (9 AM EST / 6 AM PST)
- **Async Standup**: Tuesday, Wednesday, Thursday at 2 PM UTC (9 AM EST / 6 AM PST)
- **Weekly DMs**: Mondays at 2 PM UTC (9 AM EST / 6 AM PST)
- **PR-to-Issue Linking**: Triggered when PRs are opened/edited in any org repository

You can also trigger workflows manually:

1. Go to the "Actions" tab in your repository
2. Select the workflow you want to run ("Triage Stale Issues" or "Detect Duplicate Issues")
3. Click "Run workflow"

## Project Structure

```
project-agent/
├── cmd/
│   ├── triage-stale/
│   │   └── main.go                  # Stale issue triage command
│   ├── detect-duplicates/
│   │   └── main.go                  # Duplicate detection command
│   ├── process-initiatives/
│   │   └── main.go                  # Initiative processing command
│   ├── link-pr/
│   │   └── main.go                  # PR-to-issue linking command
│   ├── scan-open-prs/
│   │   └── main.go                  # Scan all open PRs across org
│   ├── check-daily-updates/
│   │   └── main.go                  # Daily update check with Discord
│   ├── async-standup/
│   │   └── main.go                  # Async standup thread creator
│   ├── send-weekly-dms/
│   │   └── main.go                  # Weekly DM distribution
│   └── deploy-pr-workflow/
│       └── main.go                  # Mass deployment tool
├── internal/
│   ├── tasks/
│   │   ├── stale_triage.go          # Stale issue triage logic
│   │   ├── duplicate_detection.go   # Duplicate detection logic
│   │   ├── process_initiatives.go   # Initiative processing logic
│   │   ├── pr_linking.go            # PR-to-issue linking logic
│   │   ├── daily_updates.go         # Daily update check logic
│   │   ├── async_standup.go         # Async standup thread logic
│   │   └── weekly_dms.go            # Weekly DM distribution logic
│   ├── config/
│   │   └── config.go                # Configuration management
│   ├── github/
│   │   └── client.go                # GitHub GraphQL client
│   ├── similarity/
│   │   └── client.go                # Gemini AI similarity detector
│   ├── discord/
│   │   └── client.go                # Discord bot/webhook client
│   └── parser/
│       └── issue_refs.go            # Issue reference parser
├── .github/
│   └── workflows/
│       ├── triage-stale.yml         # Daily stale triage workflow
│       ├── detect-duplicates.yml    # Weekly duplicate detection workflow
│       ├── process-initiatives.yml  # Daily initiative processing workflow
│       ├── check-daily-updates.yml  # Daily update check workflow
│       ├── async-standup.yml        # Async standup workflow (Tue/Wed/Thu)
│       ├── send-weekly-dms.yml      # Weekly DM distribution workflow
│       └── handle-pr-link.yml       # PR linking receiver (repository_dispatch)
├── Makefile                         # Build automation
├── CLAUDE.md                        # Instructions for Claude Code
└── README.md
```

## Expected Status Values

The agent expects your GitHub Project to have the following Status field values:
- **Inbox** - New issues
- **Backlog** - Issues to be worked on
- **Sprint Backlog** - Issues planned for current sprint
- **In Progress** - Issues actively being worked on
- **PR Review** - Issues with associated PRs under review
- **Stuck / Dead Issue** - Where stale issues are moved

If your project uses different status names, you can configure the target statuses via environment variables (see Configuration section) or update the code in:
- `internal/github/client.go` - `MoveToPRReview()` function
- `internal/github/client.go` - `MoveToStuckDead()` function

## Costs

### GitHub API
- Free for public repositories
- Uses GraphQL API (more efficient than REST)
- Respects rate limits with 2-second delays between operations

### Gemini API
- Free tier: 15 requests per minute, 1,500 requests per day
- For a backlog of 100 issues, expect ~5,000 comparisons (worst case)
- Costs depend on your usage tier
- Optimized by truncating issue bodies to 500 characters

## Troubleshooting

### "Failed to fetch project metadata"
- Ensure your `GITHUB_TOKEN` has `project` scope
- Verify the `PROJECT_NUMBER` is correct
- Check that the token has access to the organization

### "Could not find Status field in project"
- Verify your project has a "Status" field (case-sensitive)
- Check that the field type is "Single select"

### "No issues found in Backlog"
- Verify issues are added to the project
- Check that the Status field is set to "Backlog"
- Issues must be in the project, not just the repository

### Duplicate detection not working
- Verify `GEMINI_API_KEY` is set correctly
- Check Gemini API quota limits
- Reduce `DUPLICATE_SIMILARITY` threshold for more matches

## Development

### Running Tests
```bash
go test ./...
```

### Building
```bash
# Build all commands with Makefile
make build

# Or build individual commands
go build -o bin/triage-stale cmd/triage-stale/main.go
go build -o bin/detect-duplicates cmd/detect-duplicates/main.go
go build -o bin/process-initiatives cmd/process-initiatives/main.go
go build -o bin/link-pr cmd/link-pr/main.go
go build -o bin/scan-open-prs cmd/scan-open-prs/main.go
go build -o bin/check-daily-updates cmd/check-daily-updates/main.go
go build -o bin/async-standup cmd/async-standup/main.go
go build -o bin/send-weekly-dms cmd/send-weekly-dms/main.go
go build -o bin/deploy-pr-workflow cmd/deploy-pr-workflow/main.go

# Or build all with shell loop
mkdir -p bin
for cmd in cmd/*/; do
  name=$(basename $cmd)
  go build -o bin/$name $cmd/main.go
done
```

### Adding New Commands

To add a new maintenance task:

1. **Create a new task module** in `internal/tasks/`:
   ```go
   // internal/tasks/my_task.go
   package tasks

   import (
       "context"
       "github.com/storacha/project-agent/internal/config"
       "github.com/storacha/project-agent/internal/github"
   )

   type MyTaskReport struct {
       // ... report fields
   }

   func RunMyTask(ctx context.Context, client *github.Client, issues []github.Issue, cfg *config.Config) (*MyTaskReport, error) {
       // ... task logic
   }
   ```

2. **Create a new command** in `cmd/my-task/main.go`:
   ```go
   package main

   import (
       "context"
       "log"
       "github.com/storacha/project-agent/internal/config"
       "github.com/storacha/project-agent/internal/github"
       "github.com/storacha/project-agent/internal/tasks"
   )

   func main() {
       ctx := context.Background()
       cfg, _ := config.LoadFromEnv()
       client, _ := github.NewClient(cfg.GithubToken, cfg.GithubOrg, cfg.ProjectNumber)

       issues, _ := client.GetBacklogIssues(ctx)
       report, _ := tasks.RunMyTask(ctx, client, issues, cfg)

       // ... print report
   }
   ```

3. **Create a GitHub Actions workflow** in `.github/workflows/my-task.yml`:
   ```yaml
   name: My Task
   on:
     schedule:
       - cron: '0 12 * * 3'  # Wednesdays at noon
     workflow_dispatch:

   jobs:
     my-task:
       runs-on: ubuntu-latest
       steps:
         - uses: actions/checkout@v4
         - uses: actions/setup-go@v5
           with:
             go-version: '1.22'
         - run: go run cmd/my-task/main.go
           env:
             GITHUB_TOKEN: ${{ secrets.PROJECT_MAINTENANCE_TOKEN }}
             GITHUB_ORG: storacha
             PROJECT_NUMBER: 1
   ```

### Common Task Ideas

- **Priority Adjustment**: Auto-prioritize issues based on activity/age
- **Label Cleanup**: Remove outdated or conflicting labels
- **Milestone Management**: Auto-assign issues to milestones
- **Weekly Digest**: Generate summary reports
- **Dependency Updates**: Track and label dependency-related issues
- **Comment Cleanup**: Archive or hide old automated comments

## License

MIT

## Contributing

Contributions welcome! Please open an issue or PR.

## Support

For issues, questions, or suggestions, please [open an issue](https://github.com/storacha/project-agent/issues).
