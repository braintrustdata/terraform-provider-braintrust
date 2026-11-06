package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"

	"github.com/braintrustdata/braintrust-sdk-go/openapi"
	"github.com/google/uuid"
)

type CreateAPIKeyRequest struct {
	Name             string  `json:"name"`
	OrgID            *string `json:"org_id,omitempty"`
	ExpiresInSeconds *int64  `json:"expires_in_seconds,omitempty"`
}

type CreateAPIKeyResponse struct {
	openapi.ApiKey
	Key string `json:"key"`
}

func (c *Client) CreateAPIKey(ctx context.Context, input CreateAPIKeyRequest) (*CreateAPIKeyResponse, error) {
	body, err := json.Marshal(input)
	if err != nil {
		return nil, &RequestNotSentError{Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.baseURL, "/")+"/v1/api_key", bytes.NewReader(body))
	if err != nil {
		return nil, &RequestNotSentError{Cause: err}
	}
	req.Header.Set("Content-Type", JSONContentType)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	var result CreateAPIKeyResponse
	decodeErr := json.NewDecoder(resp.Body).Decode(&result)
	if err := RejectExisting(resp, "API key"); err != nil {
		return nil, err
	}
	contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if decodeErr != nil || contentType != JSONContentType || result.Id == uuid.Nil || result.Key == "" {
		return &result, &UncertainWriteError{Method: http.MethodPost, Cause: errors.New("API key creation returned an incomplete or invalid response")}
	}
	return &result, nil
}
