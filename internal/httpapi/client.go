package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"g7.mph.tech/mph-tech/autodev/internal/model"
)

type Client struct {
	baseURL string
	client  *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{},
	}
}

func (c *Client) Enqueue() error {
	return c.post("/enqueue", nil, nil)
}

func (c *Client) Reconcile() error {
	return c.post("/reconcile", nil, nil)
}

func (c *Client) Recover() error {
	return c.post("/recover", nil, nil)
}

func (c *Client) Snapshot() (model.PersistedState, error) {
	resp, err := c.client.Get(c.baseURL + "/snapshot")
	if err != nil {
		return model.PersistedState{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return model.PersistedState{}, fmt.Errorf("snapshot failed: %s", string(body))
	}
	var state model.PersistedState
	if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
		return model.PersistedState{}, err
	}
	return state, nil
}

func (c *Client) Claim(workerID string, leaseSeconds int, allowedStages []string) (*model.StageAttempt, model.StageSpec, error) {
	body := map[string]any{
		"worker_id":      workerID,
		"lease_seconds":  leaseSeconds,
		"allowed_stages": allowedStages,
	}
	reqBody, err := json.Marshal(body)
	if err != nil {
		return nil, model.StageSpec{}, err
	}
	resp, err := c.client.Post(c.baseURL+"/attempts/claim", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		return nil, model.StageSpec{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return nil, model.StageSpec{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		return nil, model.StageSpec{}, fmt.Errorf("claim failed: %s", string(data))
	}
	var payload struct {
		Attempt model.StageAttempt `json:"attempt"`
		Spec    model.StageSpec    `json:"spec"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, model.StageSpec{}, err
	}
	return &payload.Attempt, payload.Spec, nil
}

func (c *Client) Heartbeat(attemptID string, leaseSeconds int, summary string, metadata map[string]any) error {
	body := map[string]any{
		"lease_seconds": leaseSeconds,
		"summary":       summary,
		"metadata":      metadata,
	}
	return c.post("/attempts/"+attemptID+"/heartbeat", body, nil)
}

func (c *Client) Complete(attemptID string, result model.StageResult, artifacts []model.ArtifactRef) error {
	body := map[string]any{
		"result":    result,
		"artifacts": artifacts,
	}
	return c.post("/attempts/"+attemptID+"/complete", body, nil)
}

func (c *Client) post(path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	resp, err := c.client.Post(c.baseURL+path, "application/json", reader)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		data, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request %s failed: %s", path, string(data))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
