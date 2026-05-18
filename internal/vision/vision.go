// Package vision wraps the local Ollama HTTP API to ask SmolVLM2 yes/no
// questions about screenshots, plus narrate and summarize runs.
package vision

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	functionalPrompt = "You are a QA test assistant. Look at the screenshot of a web application. " +
		"Answer the following question with 'YES' or 'NO' as the FIRST word, then a " +
		"one-sentence reason. Question: %s"
	narratePrompt = "In ONE short sentence, describe what is visible on this web app screenshot. " +
		"Mention specific UI elements (buttons, forms, errors, content). No preamble."
	summaryPrompt = "You are a senior QA engineer. Below is a per-step log from one regression " +
		"run. Write 2-3 sentences summarizing what was tested, what worked, and what failed. " +
		"Be specific. No bullets, no markdown.\n\nRun log:\n%s"
)

type Client struct {
	host   string
	model  string
	client *http.Client
}

func NewClient(host, model string, timeout time.Duration) *Client {
	return &Client{
		host:   strings.TrimRight(host, "/"),
		model:  model,
		client: &http.Client{Timeout: timeout},
	}
}

// chatRequest is the subset of the Ollama /api/chat payload we use.
type chatRequest struct {
	Model    string                   `json:"model"`
	Messages []map[string]interface{} `json:"messages"`
	Options  map[string]interface{}   `json:"options"`
	Stream   bool                     `json:"stream"`
}

type chatResponse struct {
	Message struct {
		Content string `json:"content"`
	} `json:"message"`
}

// ask sends a single-message chat call with optional images.
func (c *Client) ask(prompt string, imagePaths []string) (string, error) {
	images := make([]string, 0, len(imagePaths))
	for _, p := range imagePaths {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("read %s: %w", p, err)
		}
		images = append(images, base64.StdEncoding.EncodeToString(b))
	}
	body := chatRequest{
		Model: c.model,
		Messages: []map[string]interface{}{
			{"role": "user", "content": prompt, "images": images},
		},
		Options: map[string]interface{}{
			"temperature": 0.0,
			"num_predict": 200,
		},
		Stream: false,
	}
	buf, _ := json.Marshal(body)
	resp, err := c.client.Post(c.host+"/api/chat", "application/json", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ollama returned %d", resp.StatusCode)
	}
	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	return strings.TrimSpace(out.Message.Content), nil
}

// Ping returns true if Ollama is reachable.
func (c *Client) Ping() bool {
	req, _ := http.NewRequest("GET", c.host+"/api/version", nil)
	resp, err := c.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == 200
}

// Verdict is the parsed outcome of a yes/no AI check.
type Verdict struct {
	Passed bool
	Raw    string
}

// FunctionalCheck asks the model a yes/no question about a screenshot.
func (c *Client) FunctionalCheck(imagePath, question string, expectYes bool) (Verdict, error) {
	raw, err := c.ask(fmt.Sprintf(functionalPrompt, question), []string{imagePath})
	if err != nil {
		return Verdict{Raw: err.Error()}, err
	}
	first := strings.ToUpper(strings.TrimRight(strings.Fields(raw+" ")[0], ".,:;!?"))
	gotYes := strings.HasPrefix(first, "YES")
	return Verdict{Passed: gotYes == expectYes, Raw: raw}, nil
}

// Narrate returns one short sentence describing what's on screen.
func (c *Client) Narrate(imagePath string) (string, error) {
	raw, err := c.ask(narratePrompt, []string{imagePath})
	if err != nil {
		return "", err
	}
	// Trim to one sentence if the model rambled.
	if idx := strings.IndexAny(raw, "\n"); idx > 0 {
		raw = raw[:idx]
	}
	return strings.TrimSpace(raw), nil
}

// SummarizeRun produces a 2-3 sentence verdict for an entire run log.
func (c *Client) SummarizeRun(runLog string) (string, error) {
	if len(runLog) > 3000 {
		runLog = runLog[len(runLog)-3000:]
	}
	return c.ask(fmt.Sprintf(summaryPrompt, runLog), nil)
}
