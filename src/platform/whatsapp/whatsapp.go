// Package whatsapp sends template messages through Meta's Cloud API.
// Only the approved template name and its parameters travel: free text
// outside the 24-hour window is rejected by Meta, by design. A SIM
// gateway or a BSP would implement notify.Sender the same way, so the
// provider swaps without touching the domain.
package whatsapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"grade/src/core/notify"
)

// Client is a notify.Sender over the WhatsApp Cloud API.
type Client struct {
	http     *http.Client
	token    string
	phoneID  string
	apiBase  string
	maxBytes int64
}

// NewClient wires a sender. apiBase defaults to Meta's graph endpoint;
// token is the permanent system-user token, phoneID the business number.
func NewClient(token, phoneID, apiBase string) (*Client, error) {
	if strings.TrimSpace(token) == "" || strings.TrimSpace(phoneID) == "" {
		return nil, fmt.Errorf("whatsapp: token and phone id are required")
	}
	if strings.TrimSpace(apiBase) == "" {
		apiBase = "https://graph.facebook.com/v21.0"
	}
	return &Client{
		http:     &http.Client{Timeout: 20 * time.Second},
		token:    token,
		phoneID:  phoneID,
		apiBase:  strings.TrimRight(strings.TrimSpace(apiBase), "/"),
		maxBytes: 1 << 20,
	}, nil
}

type templateParam struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type templatePayload struct {
	Name     string `json:"name"`
	Language struct {
		Code string `json:"code"`
	} `json:"language"`
	Components []struct {
		Type       string          `json:"type"`
		Parameters []templateParam `json:"parameters"`
	} `json:"components,omitempty"`
}

type sendRequest struct {
	MessagingProduct string          `json:"messaging_product"`
	To               string          `json:"to"`
	Type             string          `json:"type"`
	Template         templatePayload `json:"template"`
}

// Send implements notify.Sender.
func (c *Client) Send(ctx context.Context, n notify.Notification) error {
	if c == nil {
		return fmt.Errorf("whatsapp: not configured")
	}
	params := make([]templateParam, 0, len(n.Params))
	for _, p := range n.Params {
		params = append(params, templateParam{Type: "text", Text: p})
	}
	tpl := templatePayload{Name: n.Template}
	tpl.Language.Code = n.Language
	if tpl.Language.Code == "" {
		tpl.Language.Code = "es"
	}
	if len(params) > 0 {
		tpl.Components = []struct {
			Type       string          `json:"type"`
			Parameters []templateParam `json:"parameters"`
		}{{Type: "body", Parameters: params}}
	}
	body, err := json.Marshal(sendRequest{
		MessagingProduct: "whatsapp",
		To:               n.To,
		Type:             "template",
		Template:         tpl,
	})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.apiBase+"/"+c.phoneID+"/messages", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	limited, _ := io.ReadAll(io.LimitReader(resp.Body, c.maxBytes))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("whatsapp: status %d: %s", resp.StatusCode, strings.TrimSpace(string(limited)))
	}
	return nil
}
