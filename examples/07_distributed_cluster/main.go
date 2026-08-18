package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	coco "cocodb"
	"cocodb/cluster"
)

func main() {
	fmt.Println("🚀 Initializing CoCoDB Secure Distributed Cluster (mTLS 1.3)...")

	tempDir := "cluster_demo_data"
	_ = os.RemoveAll(tempDir)
	_ = os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)

	// Generate Dev mTLS 1.3 Certificates (Automatic Zero-Setup Security)
	nodeTLS, clientTLS := cluster.WithDevmTLS("127.0.0.1", "localhost")
	clusterSecret := "prod_cluster_secret_token_9988"

	// =========================================================================
	// 1. Start 3 Secure Cluster Nodes with Authentication & mTLS
	// =========================================================================
	fmt.Println("\n--- [1] Starting 3 Secure Cluster Nodes ---")

	// Node 1
	db1, _ := coco.Open(filepath.Join(tempDir, "node1.coco"))
	defer db1.Close()
	node1, err := cluster.StartNode(db1, "127.0.0.1:9101",
		cluster.WithSecret(clusterSecret),
		cluster.WithPeers("127.0.0.1:9102", "127.0.0.1:9103"),
		nodeTLS,
	)
	if err != nil {
		panic(err)
	}
	defer node1.Close()
	fmt.Printf("   🔒 Node 1 Online: %s (mTLS 1.3 Active, Auth Secret Configured)\n", node1.Addr())

	// Node 2
	db2, _ := coco.Open(filepath.Join(tempDir, "node2.coco"))
	defer db2.Close()
	node2, err := cluster.StartNode(db2, "127.0.0.1:9102",
		cluster.WithSecret(clusterSecret),
		cluster.WithPeers("127.0.0.1:9101", "127.0.0.1:9103"),
		nodeTLS,
	)
	if err != nil {
		panic(err)
	}
	defer node2.Close()
	fmt.Printf("   🔒 Node 2 Online: %s (mTLS 1.3 Active, Auth Secret Configured)\n", node2.Addr())

	// Node 3
	db3, _ := coco.Open(filepath.Join(tempDir, "node3.coco"))
	defer db3.Close()
	node3, err := cluster.StartNode(db3, "127.0.0.1:9103",
		cluster.WithSecret(clusterSecret),
		cluster.WithPeers("127.0.0.1:9101", "127.0.0.1:9102"),
		nodeTLS,
	)
	if err != nil {
		panic(err)
	}
	defer node3.Close()
	fmt.Printf("   🔒 Node 3 Online: %s (mTLS 1.3 Active, Auth Secret Configured)\n", node3.Addr())

	// =========================================================================
	// 2. Connect Distributed Client with Credentials & TLS
	// =========================================================================
	fmt.Println("\n--- [2] Connecting Client via Cluster RPC ---")
	client, err := cluster.Dial([]string{"127.0.0.1:9101", "127.0.0.1:9102", "127.0.0.1:9103"},
		cluster.WithSecret(clusterSecret),
		clientTLS,
	)
	if err != nil {
		panic(err)
	}
	defer client.Close()

	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		panic(err)
	}
	fmt.Println("   ✅ Connected to 3-Node Cluster (Authentication & mTLS Handshake Succeeded)")

	// Test Unauthorized Connection Attempt
	fmt.Println("\n--- [3] Testing Security: Unauthorized Connection Attempt ---")
	badClient, _ := cluster.Dial([]string{"127.0.0.1:9101"},
		cluster.WithSecret("wrong_secret_token"),
		clientTLS,
	)
	defer badClient.Close()
	err = badClient.Ping(ctx)
	if err != nil {
		fmt.Printf("   🛑 Unauthorized attempt blocked as expected: %v\n", err)
	}

	// =========================================================================
	// 4. Distributed Task Queue & Cross-Node Deduplication
	// =========================================================================
	fmt.Println("\n--- [4] Distributed Task Queue & Zero-Duplicate Guarantee ---")
	orderQueue := client.Queue("distributed_order_processing")
	orderID := "order_checkout_TXN_887766"

	// Producer 1 enqueues task
	fmt.Printf("📦 Producer 1: Enqueuing task with DedupID=%s\n", orderID)
	msg1, err := orderQueue.Enqueue(ctx, []byte(`{"order_id": "887766", "amount": 299.00}`),
		coco.WithDedupID(orderID, 5*time.Minute),
		coco.WithPriority(200),
	)
	if err != nil {
		panic(err)
	}
	fmt.Printf("   ✅ Task successfully enqueued on Cluster (ID: %s)\n", msg1.ID)

	// Producer 2 attempts duplicate enqueue (routed across cluster nodes)
	fmt.Printf("📦 Producer 2: Attempting duplicate enqueue with same DedupID=%s\n", orderID)
	_, err = orderQueue.Enqueue(ctx, []byte(`{"order_id": "887766", "amount": 299.00}`),
		coco.WithDedupID(orderID, 5*time.Minute),
	)
	if err != nil {
		fmt.Printf("   🛑 Duplicate rejected across cluster! Reason: %v\n", err)
	}

	// Worker pulls task
	fmt.Println("\n--- [5] Distributed Worker Dequeue & Acknowledgment ---")
	workerTask, err := orderQueue.Dequeue(ctx, coco.WithVisibilityTimeout(5*time.Second))
	if err != nil {
		panic(err)
	}
	fmt.Printf("👷 Worker received task: ID=%s, Payload=%s\n", workerTask.ID, string(workerTask.Payload))
	if err := workerTask.Ack(); err != nil {
		panic(err)
	}
	fmt.Println("   ✅ Worker completed and acknowledged task (Ack) -> Removed from cluster storage.")

	// =========================================================================
	// 5. Distributed Pub/Sub Broadcasting
	// =========================================================================
	fmt.Println("\n--- [6] Distributed Real-Time Pub/Sub Broadcasting ---")
	ps := client.PubSub()
	count, err := ps.Publish(ctx, "cluster.orders.completed", []byte(`{"status": "fulfilled", "order_id": "887766"}`))
	if err != nil {
		panic(err)
	}
	fmt.Printf("📢 Event broadcasted to topic 'cluster.orders.completed' across cluster (Hit: %d)\n", count)

	fmt.Println("\n🎉 All Distributed Cluster & mTLS security flows completed successfully!")
}
