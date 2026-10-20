package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path"
	"time"
)

// PCF is a client for Npcf_PolicyAuthorization (TS 29.514), the N5 interface
// an application function uses to ask the 5G core for QoS.
type PCF struct {
	Root   string // e.g. http://127.0.0.7:8000
	Client *http.Client
}

const appSessions = "/npcf-policyauthorization/v1/app-sessions"

// Create posts an AppSessionContext and returns the app session ID from the
// Location header.
func (p *PCF) Create(ctx AppSessionContext) (string, error) {
	body, _ := json.Marshal(ctx)
	resp, err := p.post(p.Root+appSessions, body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("PCF answered %s: %s", resp.Status, resp.body)
	}
	return path.Base(resp.Header.Get("Location")), nil
}

// Delete removes the app session; the PCF then removes its PCC rule from the SMF.
func (p *PCF) Delete(id string) error {
	resp, err := p.post(p.Root+appSessions+"/"+id+"/delete", []byte("{}"))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PCF answered %s: %s", resp.Status, resp.body)
	}
	return nil
}

type response struct {
	*http.Response
	body string
}

func (p *PCF) post(url string, body []byte) (*response, error) {
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return &response{resp, string(b)}, nil
}
