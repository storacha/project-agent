COMMANDS=bin/async-standup bin/check-daily-updates bin/deploy-pr-workflow bin/detect-duplicates bin/link-pr bin/process-initiatives bin/scan-open-prs bin/send-weekly-dms bin/triage-stale

.PHONY: $(COMMANDS)

$(COMMANDS): bin/%:
	go build -o $@ ./cmd/$*

build: $(COMMANDS)