package event

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCninfoEvidenceTargetResolverResolvesAndCaches(
	t *testing.T,
) {
	var calls atomic.Int32
	client := httpDoerFunc(
		func(
			req *http.Request,
		) (*http.Response, error) {
			calls.Add(1)
			if req.URL.String() !=
				cninfoStockIndexURL {
				t.Fatalf(
					"URL = %q",
					req.URL.String(),
				)
			}
			return jsonHTTPResponse(
				http.StatusOK,
				map[string]any{
					"stockList": []map[string]any{
						{
							"code":  "600519",
							"orgId": "gssh0600519",
							"zwjc":  "贵州茅台",
						},
						{
							"code":  "000001",
							"orgId": "gssz0000001",
							"zwjc":  "平安银行",
						},
					},
				},
			), nil
		},
	)

	resolver := &CninfoEvidenceTargetResolver{
		Client: client,
	}

	const goroutines = 8
	results := make(
		[]EvidenceTarget,
		goroutines,
	)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	for index := 0; index < goroutines; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] =
				resolver.Resolve(
					context.Background(),
					ResearchTask{
						Symbol: "600519.SH",
					},
				)
		}(index)
	}
	wg.Wait()

	for index, err := range errs {
		if err != nil {
			t.Fatalf(
				"Resolve(%d) error = %v",
				index,
				err,
			)
		}
		target := results[index]
		if target.Symbol != "600519.SH" ||
			target.Name != "贵州茅台" ||
			len(target.Aliases) != 1 ||
			target.Aliases[0] != "600519" {
			t.Fatalf(
				"target %d = %+v",
				index,
				target,
			)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf(
			"stock-index HTTP calls = %d, want 1",
			got,
		)
	}

	second, err := resolver.Resolve(
		context.Background(),
		ResearchTask{
			Symbol: "000001.SZ",
		},
	)
	if err != nil {
		t.Fatalf(
			"cached Resolve() error = %v",
			err,
		)
	}
	if second.Name != "平安银行" {
		t.Fatalf(
			"cached target = %+v",
			second,
		)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf(
			"cached HTTP calls = %d, want 1",
			got,
		)
	}
}

func TestCninfoEvidenceTargetResolverRejectsUnknownSymbol(
	t *testing.T,
) {
	resolver := &CninfoEvidenceTargetResolver{
		Client: httpDoerFunc(
			func(
				*http.Request,
			) (*http.Response, error) {
				return jsonHTTPResponse(
					http.StatusOK,
					map[string]any{
						"stockList": []map[string]any{},
					},
				), nil
			},
		),
	}

	if _, err := resolver.Resolve(
		context.Background(),
		ResearchTask{
			Symbol: "600519.SH",
		},
	); err == nil {
		t.Fatal(
			"unknown Cninfo evidence target should fail",
		)
	}
}
