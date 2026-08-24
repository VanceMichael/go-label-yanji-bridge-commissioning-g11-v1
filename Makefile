.PHONY: test race vet build run measure

test:
	GOTOOLCHAIN=local go test ./... -count=1

race:
	GOTOOLCHAIN=local go test -race ./... -count=1

vet:
	GOTOOLCHAIN=local go vet ./...

build:
	CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...

run:
	CGO_ENABLED=0 GOTOOLCHAIN=local go run ./cmd/server

measure:
	go run ../../../../.agents/skills/go-base-project-create/scripts/measure_project.go -root . -profile compact_10 -enforce
