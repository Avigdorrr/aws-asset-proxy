package main

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Getter abstracts the S3 GetObject call for testability.
type S3Getter interface {
	GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

// handler holds the dependencies for the Lambda function.
type handler struct {
	s3Client   S3Getter
	bucketName string
}

// handleRequest processes an incoming Lambda Function URL request, fetches the
// corresponding object from S3, and returns it with the correct Content-Type.
func (h *handler) handleRequest(ctx context.Context, req events.LambdaFunctionURLRequest) (events.LambdaFunctionURLResponse, error) {
	// Extract the object key from the URL path.
	key := strings.TrimPrefix(req.RawPath, "/")
	if key == "" {
		return errorResponse(http.StatusBadRequest, "missing object key in path"), nil
	}

	out, err := h.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &h.bucketName,
		Key:    &key,
	})
	if err != nil {
		if isNotFound(err) {
			return errorResponse(http.StatusNotFound, fmt.Sprintf("object %q not found", key)), nil
		}
		return errorResponse(http.StatusInternalServerError, "failed to fetch object"), nil
	}
	defer out.Body.Close()

	body, err := io.ReadAll(out.Body)
	if err != nil {
		return errorResponse(http.StatusInternalServerError, "failed to read object body"), nil
	}

	contentType := "application/octet-stream"
	if out.ContentType != nil {
		contentType = *out.ContentType
	}

	// Lambda Function URL responses with binary data must be base64-encoded.
	return events.LambdaFunctionURLResponse{
		StatusCode:      http.StatusOK,
		Headers:         map[string]string{"Content-Type": contentType},
		Body:            base64.StdEncoding.EncodeToString(body),
		IsBase64Encoded: true,
	}, nil
}

// isNotFound checks whether the error indicates the S3 object does not exist.
func isNotFound(err error) bool {
	var nsk *types.NoSuchKey
	if errors.As(err, &nsk) {
		return true
	}
	// SDK v2 may return a generic API error with code "NoSuchKey" or HTTP 404.
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		if apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NotFound" {
			return true
		}
	}
	var respErr interface{ HTTPStatusCode() int }
	if errors.As(err, &respErr) && respErr.HTTPStatusCode() == http.StatusNotFound {
		return true
	}
	return strings.Contains(err.Error(), "NoSuchKey")
}

func errorResponse(status int, msg string) events.LambdaFunctionURLResponse {
	return events.LambdaFunctionURLResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "text/plain"},
		Body:       msg,
	}
}

func main() {
	bucketName := os.Getenv("BUCKET_NAME")
	if bucketName == "" {
		panic("BUCKET_NAME environment variable is required")
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		panic(fmt.Sprintf("unable to load AWS config: %v", err))
	}

	var s3Opts []func(*s3.Options)
	// Enable path-style addressing for LocalStack / local testing.
	if os.Getenv("AWS_ENDPOINT_URL") != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	h := &handler{
		s3Client:   s3.NewFromConfig(cfg, s3Opts...),
		bucketName: bucketName,
	}
	lambda.Start(h.handleRequest)
}
