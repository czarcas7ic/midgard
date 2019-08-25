all: lint install

install: go.sum
	GO111MODULE=on go install -v ./cmd/etl

go.sum: go.mod
	@echo "--> Ensure dependencies have not been modified"
	GO111MODULE=on go mod verify

lint-pre:
	@test -z $(gofmt -l .) # checks code is in proper format
	@go mod verify

lint: lint-pre
	@golangci-lint run

lint-verbose: lint-pre
	@golangci-lint run -v

build:
	@go build ./...

clean:
	rm ${GOBIN}/etl

test-coverage:
	@go test -mod=readonly -v -coverprofile .testCoverage.txt ./...

coverage-report: test-coverage
	@tool cover -html=.testCoverage.txt

test:
	@go test -mod=readonly ./...

clear:
	clear

test-watch: clear
	@./scripts/watch.bash
