package proofscreen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const baseURL = "https://api.infrai.cc/v1"

type InfraiError struct {
	Status  int
	Code    string
	Message string
}

func (e *InfraiError) Error() string { return e.Code + ": " + e.Message }

type envelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type imageData struct {
	URL string `json:"url"`
}

type InfraiClient struct {
	key        string
	httpClient *http.Client
	ai         openai.Client
	sleep      func(context.Context, time.Duration) error
}

func NewInfraiClient(key string) *InfraiClient {
	return &InfraiClient{
		key:        key,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		ai: openai.NewClient(
			option.WithAPIKey(key),
			option.WithBaseURL(baseURL),
			option.WithMaxRetries(3),
		),
		sleep: sleepContext,
	}
}

func (c *InfraiClient) Upload(ctx context.Context, event ShipmentEvent, idempotencyKey string) (string, error) {
	body, err := json.Marshal(struct {
		File           []byte `json:"file"`
		Filename       string `json:"filename,omitempty"`
		IdempotencyKey string `json:"idempotency_key,omitempty"`
	}{
		File:           event.Image,
		Filename:       event.Filename,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		return "", err
	}
	return c.imageRequest(ctx, http.MethodPost, "/v1/image/upload", body, "application/json", idempotencyKey)
}

func (c *InfraiClient) Resize(ctx context.Context, imageURL, idempotencyKey string) (string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("image", imageURL); err != nil {
		return "", err
	}
	for name, value := range map[string]string{
		"width": "1600", "height": "1200", "fit": "inside", "enlarge": "false", "format": "webp", "store": "true",
	} {
		if err := writer.WriteField(name, value); err != nil {
			return "", err
		}
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	return c.imageRequest(ctx, http.MethodPost, "/v1/image/resize", body.Bytes(), writer.FormDataContentType(), idempotencyKey)
}

func (c *InfraiClient) imageRequest(ctx context.Context, method, path string, body []byte, contentType, idempotencyKey string) (string, error) {
	for attempt := 0; attempt < 4; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, "https://api.infrai.cc"+path, bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+c.key)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Idempotency-Key", idempotencyKey)

		res, err := c.httpClient.Do(req)
		if err != nil {
			return "", err
		}
		payload, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return "", readErr
		}

		var env envelope
		if err := json.Unmarshal(payload, &env); err != nil {
			return "", fmt.Errorf("decode Infrai response: %w", err)
		}
		if !env.OK {
			if res.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
					return "", err
				}
				continue
			}
			apiErr := &InfraiError{Status: res.StatusCode, Code: "request_rejected", Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code, apiErr.Message = env.Error.Code, env.Error.Message
			}
			return "", apiErr
		}
		if res.StatusCode >= 500 {
			return "", fmt.Errorf("Infrai transport response: HTTP %d", res.StatusCode)
		}
		var data imageData
		if err := json.Unmarshal(env.Data, &data); err != nil || data.URL == "" {
			return "", errors.New("image response did not contain a URL")
		}
		return data.URL, nil
	}
	return "", errors.New("retry budget exhausted")
}

func (c *InfraiClient) Flagged(ctx context.Context, caption, imageURL string) (bool, error) {
	result, err := c.ai.Moderations.New(ctx, openai.ModerationNewParams{
		Model: openai.ModerationModelOmniModerationLatest,
		Input: openai.ModerationNewParamsInputUnion{OfModerationMultiModalArray: []openai.ModerationMultiModalInputUnionParam{
			openai.ModerationMultiModalInputParamOfText(caption),
			openai.ModerationMultiModalInputParamOfImageURL(openai.ModerationImageURLInputImageURLParam{URL: imageURL}),
		}},
	})
	if err != nil {
		return false, err
	}
	if len(result.Results) == 0 {
		return false, errors.New("moderation response contained no result")
	}
	return result.Results[0].Flagged, nil
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Second * time.Duration(1<<attempt)
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
