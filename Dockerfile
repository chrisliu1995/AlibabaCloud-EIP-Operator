# Build the manager binary
FROM golang:1.23 AS builder

WORKDIR /workspace

# Copy the Go Modules manifests and vendor directory
COPY go.mod go.mod
COPY go.sum go.sum
COPY vendor/ vendor/

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY internal/ internal/
COPY pkg/ pkg/

# Build using vendor
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -a -o manager main.go

# Use Alpine as minimal base image to package the manager binary
FROM alpine:3.19
RUN apk --no-cache add ca-certificates
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
