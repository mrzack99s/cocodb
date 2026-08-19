package cocodb_test

import (
	"testing"
	"time"

	coco "github.com/mrzack99s/cocodb"
)

func TestTimeSeriesLogAndIoTWorkflow(t *testing.T) {
	db, err := coco.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	base := time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)
	ids, err := db.TimeSeries("sensor-readings").WriteMany(
		coco.Point{Timestamp: base, Tags: map[string]string{"device": "kitchen-1"}, Fields: map[string]any{"temperature": 21.5}},
		coco.Point{Timestamp: base.Add(time.Minute), Tags: map[string]string{"device": "kitchen-1"}, Fields: map[string]any{"temperature": 22.5}},
		coco.Point{Timestamp: base.Add(time.Minute), Tags: map[string]string{"device": "garage-1"}, Fields: map[string]any{"temperature": 30.0}},
	)
	if err != nil || len(ids) != 3 {
		t.Fatalf("WriteMany = %v, %v", ids, err)
	}

	query := db.TimeSeries("sensor-readings").Query().Range(base, base.Add(time.Minute)).Tag("device", "kitchen-1")
	points, err := query.All()
	if err != nil || len(points) != 2 || points[1].Fields["temperature"] != 22.5 {
		t.Fatalf("All = %#v, %v", points, err)
	}
	buckets, err := query.Aggregate("temperature", time.Hour, coco.Average)
	if err != nil || len(buckets) != 1 || buckets[0].Value != 22 {
		t.Fatalf("Aggregate = %#v, %v", buckets, err)
	}

	if got := db.ListTimeSeries(); len(got) != 1 || got[0] != "sensor-readings" {
		t.Fatalf("ListTimeSeries = %v", got)
	}
	removed, err := db.TimeSeries("sensor-readings").PruneBefore(base.Add(30 * time.Second))
	if err != nil || removed != 1 {
		t.Fatalf("PruneBefore = %d, %v", removed, err)
	}
}
