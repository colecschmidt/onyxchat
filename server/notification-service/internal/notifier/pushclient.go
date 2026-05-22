package notifier

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PushTokenClient fetches push tokens from onyxchat-server's internal API.
// The X-Service-Secret header authenticates the request — the server rejects
// any call that doesn't carry the shared secret.
type PushTokenClient struct {
	serverURL  string
	secret     string
	httpClient *http.Client
}

func NewPushTokenClient(serverURL, secret string) *PushTokenClient {
	return &PushTokenClient{
		serverURL: serverURL,
		secret:    secret,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

func (c *PushTokenClient) GetPushToken(ctx context.Context, userID int64) (string, error) {
	url := fmt.Sprintf("%s/internal/users/%d/push-token", c.serverURL, userID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("X-Service-Secret", c.secret)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil // user has no push token registered
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var body struct {
		PushToken string `json:"pushToken"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return body.PushToken, nil
}
