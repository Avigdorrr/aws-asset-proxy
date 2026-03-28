# Stage 1: Build the binary
FROM --platform=$BUILDPLATFORM golang:1.22.0-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Docker Buildx automatically injects these based on the platform requested in the build command
ARG TARGETOS
ARG TARGETARCH
# Using -tags lambda.norpc builds the binary for the provided.al2023 runtime (HTTP based) and creates a smaller binary.
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} CGO_ENABLED=0 go build -tags lambda.norpc -o proxy ./cmd/proxy/main.go

# Stage 2: AWS Lambda Runtime
FROM public.ecr.aws/lambda/provided:al2023
COPY --from=builder /app/proxy ./proxy
ENTRYPOINT [ "./proxy" ]
