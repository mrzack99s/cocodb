package cocodb

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mrzack99s/cocodb/document"
)

// Point is one timestamped observation. Tags identify a source (for example,
// host, device, or severity); Fields contain the measured or logged values.
// A zero Timestamp is set to the current UTC time when the point is written.
type Point struct {
	Timestamp time.Time
	Tags      map[string]string
	Fields    map[string]any
}

// Aggregate selects the calculation performed for each time bucket.
type Aggregate uint8

const (
	Count Aggregate = iota
	Sum
	Average
	Minimum
	Maximum
)

// TimeBucket is an aggregated value over [Start, End).
type TimeBucket struct {
	Start time.Time
	End   time.Time
	Count int64
	Value float64
}

var (
	ErrInvalidTimeSeries = errors.New("coco/timeseries: series name is required")
	ErrInvalidInterval   = errors.New("coco/timeseries: interval must be positive")
)

var timeSeriesSequence uint64

// TimeSeries is a log- and IoT-oriented view over a durable document
// collection. A handle from DB writes each operation atomically; a handle from
// Tx participates in the caller's transaction.
type TimeSeries struct {
	db   *DB
	tx   *Tx
	name string
}

// TimeSeries opens a named series. Names are isolated from regular document
// collections and are persisted in the same database file.
func (db *DB) TimeSeries(name string) *TimeSeries {
	return &TimeSeries{db: db, name: name}
}

// TimeSeries opens a named series inside this transaction.
func (tx *Tx) TimeSeries(name string) *TimeSeries {
	return &TimeSeries{tx: tx, name: name}
}

// ListTimeSeries returns all named time-series collections.
func (db *DB) ListTimeSeries() []string {
	const prefix = "_ts:"
	collections := db.ListCollections()
	series := make([]string, 0)
	for _, name := range collections {
		if strings.HasPrefix(name, prefix) {
			series = append(series, strings.TrimPrefix(name, prefix))
		}
	}
	sort.Strings(series)
	return series
}

func (s *TimeSeries) collectionName() (string, error) {
	if strings.TrimSpace(s.name) == "" {
		return "", ErrInvalidTimeSeries
	}
	return "_ts:" + s.name, nil
}

// Write stores one point. Tag and field names are retained verbatim, so they
// can contain dots and are safe to use with labels such as "device.id".
func (s *TimeSeries) Write(point Point) (string, error) {
	var id string
	err := s.withWrite(func(coll *document.Collection) error {
		var err error
		id, err = writePoint(coll, point)
		return err
	})
	return id, err
}

// WriteMany stores a batch atomically when called on a DB handle (or as part
// of the existing transaction when called on a Tx handle).
func (s *TimeSeries) WriteMany(points ...Point) ([]string, error) {
	ids := make([]string, 0, len(points))
	err := s.withWrite(func(coll *document.Collection) error {
		for _, point := range points {
			id, err := writePoint(coll, point)
			if err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return nil
	})
	return ids, err
}

func writePoint(coll *document.Collection, point Point) (string, error) {
	when := point.Timestamp.UTC()
	if when.IsZero() {
		when = time.Now().UTC()
	}
	seq := atomic.AddUint64(&timeSeriesSequence, 1)
	doc := document.Document{
		"_id": fmt.Sprintf("%019d-%020d", when.UnixNano(), seq),
		"_ts": when.UnixNano(),
	}
	for key, value := range point.Tags {
		doc["_tag:"+key] = value
	}
	for key, value := range point.Fields {
		doc["_field:"+key] = value
	}
	return coll.Insert(doc)
}

func (s *TimeSeries) withWrite(fn func(*document.Collection) error) error {
	name, err := s.collectionName()
	if err != nil {
		return err
	}
	if s.tx != nil {
		return fn(s.tx.Collection(name))
	}
	if s.db == nil {
		return ErrInvalidTimeSeries
	}
	return s.db.Update(func(tx *Tx) error { return fn(tx.Collection(name)) })
}

func (s *TimeSeries) withRead(fn func(*document.Collection) error) error {
	name, err := s.collectionName()
	if err != nil {
		return err
	}
	if s.tx != nil {
		return fn(s.tx.Collection(name))
	}
	if s.db == nil {
		return ErrInvalidTimeSeries
	}
	return s.db.View(func(tx *Tx) error { return fn(tx.Collection(name)) })
}

// TimeQuery selects points by an inclusive time range and exact tag matches.
type TimeQuery struct {
	series     *TimeSeries
	start, end time.Time
	tags       map[string]string
	limit      int
	descending bool
}

// Query starts a time-range query. With no range, it returns all points.
func (s *TimeSeries) Query() *TimeQuery {
	return &TimeQuery{series: s, tags: make(map[string]string)}
}

// Range selects timestamps in the inclusive [start, end] interval. Either
// boundary may be zero to leave that side open.
func (q *TimeQuery) Range(start, end time.Time) *TimeQuery {
	q.start, q.end = start.UTC(), end.UTC()
	return q
}

// Tag requires an exact tag match.
func (q *TimeQuery) Tag(key, value string) *TimeQuery {
	q.tags[key] = value
	return q
}

// Limit caps the number of returned points.
func (q *TimeQuery) Limit(limit int) *TimeQuery { q.limit = limit; return q }

// Desc returns newest points first. Results are oldest first by default.
func (q *TimeQuery) Desc() *TimeQuery { q.descending = true; return q }

// All executes the query and returns points in timestamp order.
func (q *TimeQuery) All() ([]Point, error) {
	var points []Point
	err := q.series.withRead(func(coll *document.Collection) error {
		docs, err := q.documents(coll)
		if err != nil {
			return err
		}
		points = make([]Point, 0, len(docs))
		for _, doc := range docs {
			point, ok := pointFromDocument(doc)
			if ok {
				points = append(points, point)
			}
		}
		return nil
	})
	return points, err
}

func (q *TimeQuery) documents(coll *document.Collection) ([]document.Document, error) {
	dq := coll.Query()
	if !q.start.IsZero() {
		dq = dq.Where("_ts").Gte(q.start.UnixNano())
	}
	if !q.end.IsZero() {
		dq = dq.Where("_ts").Lte(q.end.UnixNano())
	}
	for key, value := range q.tags {
		dq = dq.Where("_tag:" + key).Eq(value)
	}
	order := document.Asc
	if q.descending {
		order = document.Desc
	}
	dq = dq.OrderBy("_ts", order)
	if q.limit > 0 {
		dq = dq.Limit(q.limit)
	}
	return dq.All()
}

func pointFromDocument(doc document.Document) (Point, bool) {
	nanos, ok := asInt64(doc["_ts"])
	if !ok {
		return Point{}, false
	}
	point := Point{Timestamp: time.Unix(0, nanos).UTC(), Tags: make(map[string]string), Fields: make(map[string]any)}
	for key, value := range doc {
		switch {
		case strings.HasPrefix(key, "_tag:"):
			if tag, ok := value.(string); ok {
				point.Tags[strings.TrimPrefix(key, "_tag:")] = tag
			}
		case strings.HasPrefix(key, "_field:"):
			point.Fields[strings.TrimPrefix(key, "_field:")] = value
		}
	}
	return point, true
}

// PruneBefore removes points older than cutoff and returns the number removed.
func (s *TimeSeries) PruneBefore(cutoff time.Time) (int, error) {
	removed := 0
	err := s.withWrite(func(coll *document.Collection) error {
		docs, err := coll.Query().Where("_ts").Lt(cutoff.UTC().UnixNano()).All()
		if err != nil {
			return err
		}
		for _, doc := range docs {
			id, _ := doc["_id"].(string)
			if id != "" {
				if err := coll.Delete(id); err != nil {
					return err
				}
				removed++
			}
		}
		return nil
	})
	return removed, err
}

// Aggregate groups numeric field values into fixed UTC intervals. Count is the
// number of points in a bucket; non-numeric values are ignored for the other
// aggregate operations.
func (q *TimeQuery) Aggregate(field string, interval time.Duration, op Aggregate) ([]TimeBucket, error) {
	if interval <= 0 {
		return nil, ErrInvalidInterval
	}
	points, err := q.All()
	if err != nil {
		return nil, err
	}
	type state struct {
		count, numeric int64
		sum, min, max  float64
	}
	states := make(map[time.Time]*state)
	for _, point := range points {
		start := point.Timestamp.UTC().Truncate(interval)
		current := states[start]
		if current == nil {
			current = &state{}
			states[start] = current
		}
		current.count++
		if value, ok := asFloat64(point.Fields[field]); ok {
			if current.numeric == 0 || value < current.min {
				current.min = value
			}
			if current.numeric == 0 || value > current.max {
				current.max = value
			}
			current.sum += value
			current.numeric++
		}
	}
	starts := make([]time.Time, 0, len(states))
	for start := range states {
		starts = append(starts, start)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })
	buckets := make([]TimeBucket, 0, len(starts))
	for _, start := range starts {
		current := states[start]
		value := current.sum
		switch op {
		case Count:
			value = float64(current.count)
		case Average:
			if current.numeric > 0 {
				value = current.sum / float64(current.numeric)
			}
		case Minimum:
			value = current.min
		case Maximum:
			value = current.max
		}
		buckets = append(buckets, TimeBucket{Start: start, End: start.Add(interval), Count: current.count, Value: value})
	}
	return buckets, nil
}

func asInt64(value any) (int64, bool) {
	switch n := value.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case int32:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}

func asFloat64(value any) (float64, bool) {
	switch n := value.(type) {
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	default:
		return 0, false
	}
}
