package main

import (
	"fmt"
	"log"
	"os"

	coco "cocodb"
	"cocodb/document"
)

func main() {
	dbPath := "doc_example.coco"
	defer os.Remove(dbPath)
	defer os.Remove(dbPath + "-wal")

	// 1. Open Database
	db, err := coco.Open(dbPath, coco.Profile(coco.Performance))
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// 2. Insert Documents in an ACID transaction
	fmt.Println("=== 1. Inserting Documents ===")
	items := []document.Document{
		{
			"_id":      "prod_01",
			"name":     "Mechanical Keyboard",
			"category": "electronics",
			"price":    129.99,
			"rating":   4.8,
			"in_stock": true,
		},
		{
			"_id":      "prod_02",
			"name":     "Wireless Mouse",
			"category": "electronics",
			"price":    49.50,
			"rating":   4.5,
			"in_stock": true,
		},
		{
			"_id":      "prod_03",
			"name":     "4K UltraHD Monitor",
			"category": "electronics",
			"price":    399.00,
			"rating":   4.9,
			"in_stock": true,
		},
		{
			"_id":      "prod_04",
			"name":     "Ergonomic Chair",
			"category": "furniture",
			"price":    280.00,
			"rating":   4.6,
			"in_stock": false,
		},
	}

	err = db.Update(func(tx *coco.Tx) error {
		col := tx.Collection("products")
		for _, it := range items {
			if _, err := col.Insert(it); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatalf("Insert failed: %v", err)
	}
	fmt.Printf("Inserted %d products successfully.\n", len(items))

	// 3. Query with Predicates, Sorting & Limit
	fmt.Println("\n=== 2. Executing Fluent Query ===")
	products := db.Collection("products")

	q := products.Query().
		Where("category").Eq("electronics").
		Where("price").Lt(200.0).
		OrderBy("price", coco.Desc).
		Limit(10)

	// Inspect physical Volcano execution plan
	plan, _ := q.Explain()
	fmt.Printf("Physical Execution Plan: %s\n\n", plan)

	results, err := q.All()
	if err != nil {
		log.Fatalf("Query failed: %v", err)
	}

	fmt.Printf("Found %d matching electronics under $200:\n", len(results))
	for _, doc := range results {
		fmt.Printf("  • [%s] %-22s | Price: $%.2f | Rating: %.1f★\n",
			doc["_id"], doc["name"], doc["price"], doc["rating"])
	}

	// 4. Update Document
	fmt.Println("\n=== 3. Updating Document ===")
	_ = db.Update(func(tx *coco.Tx) error {
		col := tx.Collection("products")
		doc, err := col.Get("prod_01")
		if err != nil {
			return err
		}
		doc["price"] = 119.99 // On sale
		doc["on_sale"] = true
		return col.Replace("prod_01", doc)
	})

	updatedDoc, _ := products.Get("prod_01")
	fmt.Printf("Updated prod_01: Price is now $%.2f (On Sale: %v)\n",
		updatedDoc["price"], updatedDoc["on_sale"])
}
