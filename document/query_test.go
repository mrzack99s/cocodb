package document_test

import (
	"fmt"
	"testing"

	"github.com/mrzack99s/cocodb/document"
	"github.com/mrzack99s/cocodb/internal/file"
	"github.com/mrzack99s/cocodb/internal/query/executor"
	"github.com/mrzack99s/cocodb/internal/record"
	"github.com/mrzack99s/cocodb/internal/storage"
	"github.com/mrzack99s/cocodb/internal/txn"
	"github.com/mrzack99s/cocodb/internal/types"
	"github.com/mrzack99s/cocodb/internal/wal"
)

func TestQueryEngineAndFiltering(t *testing.T) {
	memDB := file.NewMemoryBackend()
	pager, err := storage.OpenPager(memDB, 16*1024*1024, false)
	if err != nil {
		t.Fatalf("OpenPager failed: %v", err)
	}

	memWAL := file.NewMemoryBackend()
	walManager, err := wal.OpenWAL(memWAL, 0)
	if err != nil {
		t.Fatalf("OpenWAL failed: %v", err)
	}

	tm := txn.NewTxnManager(pager, walManager, txn.SyncOff)
	dir := record.NewDirectory(pager, types.InvalidPageID)
	store := record.NewStore(pager, dir, tm)

	tx, _ := tm.Begin(false)
	coll := document.NewCollection("products", 1, pager, tx, store, nil, types.InvalidPageID)

	// Insert 20 products
	for i := 1; i <= 20; i++ {
		category := "electronics"
		if i%2 == 0 {
			category = "books"
		}
		_, err := coll.Insert(document.Document{
			"_id":      fmt.Sprintf("p%02d", i),
			"name":     fmt.Sprintf("Product %02d", i),
			"category": category,
			"price":    float64(i * 10),
			"stock":    int64(i * 5),
		})
		if err != nil {
			t.Fatalf("insert %d failed: %v", i, err)
		}
	}

	// Query 1: Where category = "electronics" AND price >= 50, OrderBy price DESC, Limit 3
	rows, err := coll.Query().
		Where("category").Eq("electronics").
		Where("price").Gte(float64(50)).
		OrderBy("price", document.Desc).
		Limit(3).
		All()
	if err != nil {
		t.Fatalf("Query 1 failed: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	if rows[0]["_id"] != "p19" || rows[0]["price"] != float64(190) {
		t.Fatalf("expected p19 first with price 190, got %v", rows[0])
	}

	// Query 2: Explain plan
	plan, err := coll.Query().
		Where("category").Eq("books").
		OrderBy("price", document.Asc).
		Limit(5).
		Explain()
	if err != nil || plan == "" {
		t.Fatalf("Explain failed: %v, plan=%s", err, plan)
	}

	// Query 3: Count
	cnt, err := coll.Query().Where("category").Eq("books").Count()
	if err != nil || cnt != 10 {
		t.Fatalf("Count expected 10 books, got %d (err: %v)", cnt, err)
	}

	// Query 4: Aggregation (GroupBy category, Sum price, Avg price, Min price, Max price)
	op, _, err := coll.Query().Plan()
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}

	aggs, err := executor.ComputeAggregation(op, []string{"category"}, []executor.AggDef{
		{Type: executor.AggCount, Field: "_id", As: "total_count"},
		{Type: executor.AggSum, Field: "price", As: "total_price"},
		{Type: executor.AggAvg, Field: "price", As: "avg_price"},
		{Type: executor.AggMin, Field: "price", As: "min_price"},
		{Type: executor.AggMax, Field: "price", As: "max_price"},
	})
	if err != nil {
		t.Fatalf("ComputeAggregation failed: %v", err)
	}

	if len(aggs) != 2 {
		t.Fatalf("expected 2 category groups, got %d", len(aggs))
	}

	for _, res := range aggs {
		cat := res.GroupKey["category"].(string)
		if cat == "books" {
			if res.Values["total_count"] != int64(10) {
				t.Fatalf("books count mismatch: %v", res.Values)
			}
		}
	}
}
