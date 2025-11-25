package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type AIParsedTrack struct {
	OriginalFilename string `json:"original_filename"`
	Artist           string `json:"artist"`
	Title            string `json:"title"`
	TrackNumber      string `json:"track_number"`
}

type AIResponse struct {
	Tracks []AIParsedTrack `json:"tracks"`
}

// Gemini Request/Response structures
type GeminiRequest struct {
	Contents []GeminiContent `json:"contents"`
}

type GeminiContent struct {
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

func (a *App) ParseFilenamesWithAI(filenames []string, apiKey string) ([]AIParsedTrack, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	// Prepare the prompt
	fileList := strings.Join(filenames, "\n")
	prompt := fmt.Sprintf(`
I have a list of audio filenames that are messy. Please extract the Artist, Title, and Track Number (if present) for each.
Return the result as a JSON object with a key "tracks" containing a list of objects.
Each object should have: "original_filename", "artist", "title", "track_number".
If a field is missing, use an empty string.
Do not include any markdown formatting (like `+"```json"+`) in the response, just the raw JSON string.

Filenames:
%s
`, fileList)

	reqBody := GeminiRequest{
		Contents: []GeminiContent{
			{
				Parts: []GeminiPart{
					{Text: prompt},
				},
			},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-lite:generateContent?key=" + apiKey
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("AI API error: %s - %s", resp.Status, string(body))
	}

	var geminiResp GeminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, err
	}

	if len(geminiResp.Candidates) == 0 || len(geminiResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	content := geminiResp.Candidates[0].Content.Parts[0].Text
	// Clean up potential markdown code blocks
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)

	var result AIResponse
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w. Content: %s", err, content)
	}

	return result.Tracks, nil
}
