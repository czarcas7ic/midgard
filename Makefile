# To install prerequisits:
#
# To install redoc-cli:
# $ npm install
#
# To install oapi-codegen in $GOPATH/bin, go outside this go module:
# $ go get github.com/deepmap/oapi-codegen/cmd/oapi-codegen

all: generated
generated: oapi-doc oapi-go graphql-go

GOBIN?=${GOPATH}/bin

API_REST_SPEC=./openapi/openapi.yaml
API_REST_CODE_GEN_LOCATION=./openapi/generated/oapigen/oapigen.go
API_REST_DOCO_GEN_LOCATION=./openapi/generated/doc.html

# Open API Makefile targets
oapi-validate:
	./node_modules/.bin/oas-validate -v ${API_REST_SPEC}

oapi-go: oapi-validate
	@${GOBIN}/oapi-codegen --package oapigen --generate types,spec -o ${API_REST_CODE_GEN_LOCATION} ${API_REST_SPEC}

oapi-doc: oapi-validate
	./node_modules/.bin/redoc-cli bundle ${API_REST_SPEC} -o ${API_REST_DOCO_GEN_LOCATION}

graphql-go:
	go generate ./...

test:
	go test -p 1 -v ./...

build:
	docker build -t registry.gitlab.com/thorchain/midgard:develop .
