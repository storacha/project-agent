COMMANDS=bin/check-daily-updates bin/deploy-pr-workflow bin/detect-duplicates bin/scan-open-prs bin/send-weekly-dms bin/triage-stale

.PHONY: $(COMMANDS)

$(COMMANDS): bin/%:
	go build -o $@ ./cmd/$*

build: $(COMMANDS)