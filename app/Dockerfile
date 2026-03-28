# Stage 1: Build the binary
FROM --platform=$BUILDPLATFORM golang:1.22.0-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Build the binary targeting linux/arm64 (Graviton2).
# Using -tags lambda.norpc builds the binary for the provided.al2023 runtime (HTTP based) and creates a smaller binary.
RUN GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o proxy ./cmd/proxy/main.go

# Stage 2: AWS Lambda Runtime
FROM --platform=linux/arm64 public.ecr.aws/lambda/provided:al2023
COPY --from=builder /app/proxy ./proxy
ENTRYPOINT [ "./proxy" ]
