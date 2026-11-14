package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/syntrix/runtime/internal/security"
)

type Provider interface {
	Name() string
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

type LocalOllamaProvider struct {
	Endpoint string
	Model    string
	Client   *http.Client
}

func NewLocalOllama(model string) *LocalOllamaProvider {
	if model == "" {
		model = "llama3.2"
	}
	ep := os.Getenv("SYNTRIX_OLLAMA_URL")
	if ep == "" {
		ep = "http://127.0.0.1:11434/api/generate"
	}
	return &LocalOllamaProvider{
		Endpoint: ep,
		Model:    model,
		Client:   &http.Client{Timeout: 8 * time.Second},
	}
}

func (p *LocalOllamaProvider) Name() string {
	return "ollama:" + p.Model
}

func (p *LocalOllamaProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	body := map[string]any{
		"model":  p.Model,
		"prompt": systemPrompt + "\n\nUser: " + userPrompt,
		"stream": false,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("ollama status %d", resp.StatusCode)
	}
	var out struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return out.Response, nil
}

// CloudProvider uses an OpenAI-compatible endpoint with API key stored in macOS Keychain.
type CloudProvider struct {
	Endpoint       string
	Model          string
	KeychainAccount string
	Client         *http.Client
}

func NewCloudProvider(endpoint, model string) *CloudProvider {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &CloudProvider{
		Endpoint:        endpoint,
		Model:           model,
		KeychainAccount: "cloud_llm_api_key",
		Client:          &http.Client{Timeout: 12 * time.Second},
	}
}

func (p *CloudProvider) Name() string {
	return "cloud:" + p.Model
}

func (p *CloudProvider) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if p.Endpoint == "" {
		return "", fmt.Errorf("cloud endpoint not configured")
	}
	apiKey, err := security.GetKeychainSecret(p.KeychainAccount)
	if err != nil || apiKey == "" {
		return "", fmt.Errorf("cloud API key not set in Keychain (%v)", err)
	}
	body := map[string]any{
		"model": p.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": userPrompt},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.Endpoint, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("cloud status %d", resp.StatusCode)
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("empty cloud response")
	}
	return out.Choices[0].Message.Content, nil
}
