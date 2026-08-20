package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

func (p *Program) runPolling(ctx context.Context) {
	p.pollOnce()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.pollOnce()
		}
	}
}

func (p *Program) pollOnce() {
	attractions, err := p.fetchREST()
	if err != nil {
		log.Printf("REST poll failed: %v", err)
		return
	}
	p.applyREST(attractions, time.Now())
}

func (p *Program) fetchREST() ([]Attraction, error) {
	url := fmt.Sprintf("%s/%s/live?entityType=ATTRACTION", p.restURL, p.resortID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-API-Key", p.apiKey)
	req.Header.Set("User-Agent", "WhatThePooh-Comparison/1.0")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var response ParkLiveDataResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}

	out := make([]Attraction, 0, len(response.LiveData))
	for _, item := range response.LiveData {
		attr, ok := convertLiveDataEntity(item, 0)
		if !ok {
			continue
		}
		out = append(out, attr)
	}
	return out, nil
}
