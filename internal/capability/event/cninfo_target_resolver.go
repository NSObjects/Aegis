package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type CninfoEvidenceTargetResolver struct {
	Client HTTPDoer

	mu     sync.Mutex
	loaded bool
	names  map[string]string
}

var _ EvidenceTargetResolver = (*CninfoEvidenceTargetResolver)(nil)

func (r *CninfoEvidenceTargetResolver) Resolve(
	ctx context.Context,
	task ResearchTask,
) (EvidenceTarget, error) {
	if r == nil {
		return EvidenceTarget{}, errors.New(
			"cninfo evidence target resolver is nil",
		)
	}

	taskSymbol := strings.ToUpper(
		strings.TrimSpace(task.Symbol),
	)
	if taskSymbol == "" {
		return EvidenceTarget{}, errors.New(
			"cninfo evidence target task symbol is required",
		)
	}

	baseSymbol, err := normalizeCninfoSymbol(
		taskSymbol,
	)
	if err != nil {
		return EvidenceTarget{}, err
	}

	name, err := r.resolveName(
		ctx,
		baseSymbol,
	)
	if err != nil {
		return EvidenceTarget{}, err
	}

	return EvidenceTarget{
		Symbol: taskSymbol,
		Name:   name,
		Aliases: []string{
			baseSymbol,
		},
	}, nil
}

func (r *CninfoEvidenceTargetResolver) resolveName(
	ctx context.Context,
	baseSymbol string,
) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.loaded {
		name, ok := r.names[baseSymbol]
		if !ok {
			return "", fmt.Errorf(
				"cninfo stock index does not contain symbol %s",
				baseSymbol,
			)
		}
		return name, nil
	}

	client := r.Client
	if client == nil {
		client = &http.Client{
			Timeout: 15 * time.Second,
		}
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		cninfoStockIndexURL,
		nil,
	)
	if err != nil {
		return "", fmt.Errorf(
			"build cninfo evidence target request: %w",
			err,
		)
	}
	setCninfoHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf(
			"request cninfo evidence targets: %w",
			err,
		)
	}
	defer resp.Body.Close()

	if err := requireHTTP2xx(
		resp,
		"cninfo evidence target index",
	); err != nil {
		return "", err
	}

	var payload struct {
		StockList []struct {
			Code string `json:"code"`
			Name string `json:"zwjc"`
		} `json:"stockList"`
	}
	if err := json.NewDecoder(resp.Body).Decode(
		&payload,
	); err != nil {
		return "", fmt.Errorf(
			"decode cninfo evidence targets: %w",
			err,
		)
	}

	names := make(
		map[string]string,
		len(payload.StockList),
	)
	for _, item := range payload.StockList {
		code := strings.TrimSpace(item.Code)
		if code == "" {
			continue
		}
		names[code] = cleanCninfoText(
			item.Name,
		)
	}

	r.names = names
	r.loaded = true

	name, ok := names[baseSymbol]
	if !ok {
		return "", fmt.Errorf(
			"cninfo stock index does not contain symbol %s",
			baseSymbol,
		)
	}
	return name, nil
}
