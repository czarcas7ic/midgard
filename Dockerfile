# Build Image
FROM golang:1.15 AS build

WORKDIR /tmp/midgard

# Cache Go dependencies like this:
COPY go.mod go.sum ./
RUN go mod download

COPY  . .

# Compile.
RUN CGO_ENABLED=0 GOOS=linux go build -v -a -installsuffix cgo ./cmd/midgard

# Main Image
FROM busybox

RUN mkdir -p openapi/generated
COPY --from=build /tmp/midgard/openapi/generated/doc.html ./openapi/generated/doc.html
COPY --from=build /tmp/midgard/midgard .
COPY config/config.json .

CMD [ "./midgard", "config.json" ]
