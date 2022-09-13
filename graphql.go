package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type graphqlQuery struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

func doGraphQLQuery(ctx context.Context, url string, hc *http.Client, qreq *graphqlQuery) ([]byte, error) {
	bodyJSON, err := json.Marshal(qreq)
	if err != nil {
		return nil, err
	}
	body := bytes.NewReader(bodyJSON)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := hc.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	b, err := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: got body %q", httpResp.Status, b)
	}
	return b, err
}
