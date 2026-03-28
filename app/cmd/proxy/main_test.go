package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"testing"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// mockS3Getter implements S3Getter for testing.
type mockS3Getter struct {
	getObjectFunc func(ctx context.Context, params *s3.GetObjectInput) (*s3.GetObjectOutput, error)
}

func (m *mockS3Getter) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.getObjectFunc != nil {
		return m.getObjectFunc(ctx, params)
	}
	return nil, errors.New("unimplemented")
}

// mockAPIError implements smithy.APIError for testing isNotFound.
type mockAPIError struct {
	code    string
	message string
	fault   smithy.ErrorFault
}

func (e *mockAPIError) ErrorCode() string             { return e.code }
func (e *mockAPIError) ErrorMessage() string          { return e.message }
func (e *mockAPIError) ErrorFault() smithy.ErrorFault { return e.fault }
func (e *mockAPIError) Error() string                 { return e.code + ": " + e.message }

func TestHandleRequest(t *testing.T) {
	bucketName := "test-bucket"

	tests := []struct {
		name           string
		reqPath        string
		mockS3Return   *s3.GetObjectOutput
		mockS3Err      error
		expectedStatus int
		expectedBody   string
		expectedIsB64  bool
	}{
		{
			name:    "Successful request",
			reqPath: "/valid/path/file.txt",
			mockS3Return: &s3.GetObjectOutput{
				Body:        io.NopCloser(bytes.NewReader([]byte("hello world"))),
				ContentType: ptr("text/plain"),
			},
			expectedStatus: http.StatusOK,
			expectedBody:   base64.StdEncoding.EncodeToString([]byte("hello world")),
			expectedIsB64:  true,
		},
		{
			name:           "Empty path",
			reqPath:        "/",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "missing object key in path",
			expectedIsB64:  false,
		},
		{
			name:    "Object not found (NoSuchKey)",
			reqPath: "/missing.txt",
			mockS3Err: &types.NoSuchKey{
				Message: ptr("The specified key does not exist."),
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   "object \"missing.txt\" not found",
			expectedIsB64:  false,
		},
		{
			name:    "Object not found (API Error)",
			reqPath: "/missing2.txt",
			mockS3Err: &mockAPIError{
				code: "NotFound",
			},
			expectedStatus: http.StatusNotFound,
			expectedBody:   "object \"missing2.txt\" not found",
			expectedIsB64:  false,
		},
		{
			name:           "Internal server error",
			reqPath:        "/error.txt",
			mockS3Err:      errors.New("generic error"),
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "failed to fetch object",
			expectedIsB64:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockS3 := &mockS3Getter{
				getObjectFunc: func(ctx context.Context, params *s3.GetObjectInput) (*s3.GetObjectOutput, error) {
					if *params.Bucket != bucketName {
						t.Errorf("expected bucket %q, got %q", bucketName, *params.Bucket)
					}
					// removing leading slash to compare with key sent to mock
					expectedKey := tt.reqPath[1:]
					if *params.Key != expectedKey {
						t.Errorf("expected key %q, got %q", expectedKey, *params.Key)
					}
					return tt.mockS3Return, tt.mockS3Err
				},
			}

			h := &handler{
				s3Client:   mockS3,
				bucketName: bucketName,
			}

			req := events.LambdaFunctionURLRequest{
				RawPath: tt.reqPath,
			}

			resp, err := h.handleRequest(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if resp.StatusCode != tt.expectedStatus {
				t.Errorf("expected status %d, got %d", tt.expectedStatus, resp.StatusCode)
			}

			if resp.Body != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, resp.Body)
			}

			if resp.IsBase64Encoded != tt.expectedIsB64 {
				t.Errorf("expected IsBase64Encoded %v, got %v", tt.expectedIsB64, resp.IsBase64Encoded)
			}
		})
	}
}

func ptr(s string) *string {
	return &s
}
