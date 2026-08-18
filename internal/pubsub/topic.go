package pubsub

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/mrzack99s/cocodb/internal/queue"
)

// BackpressureStrategy defines behavior when subscriber channel is full.
type BackpressureStrategy uint8

const (
	BlockOnFull BackpressureStrategy = iota
	DropOldest
	DropNewest
)

// Message represents an event published to a Pub/Sub topic.
type Message struct {
	ID        string
	Topic     string
	Payload   []byte
	DedupID   string
	CreatedAt int64
}

// Subscription represents an active subscriber stream.
type Subscription struct {
	id         uint64
	topic      string
	group      string // Empty for direct broadcast; non-empty for consumer group
	ch         chan *Message
	strategy   BackpressureStrategy
	unsubFn    func()
	closed     atomic.Bool
}

func (s *Subscription) Channel() <-chan *Message {
	return s.ch
}

func (s *Subscription) Unsubscribe() {
	if s.closed.CompareAndSwap(false, true) {
		if s.unsubFn != nil {
			s.unsubFn()
		}
	}
}

// ConsumerGroup manages competing subscribers sharing the same group name.
type consumerGroup struct {
	mu          sync.Mutex
	name        string
	subscribers []*Subscription
	roundRobin  uint64
}

func (cg *consumerGroup) add(sub *Subscription) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	cg.subscribers = append(cg.subscribers, sub)
}

func (cg *consumerGroup) remove(subID uint64) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	filtered := cg.subscribers[:0]
	for _, s := range cg.subscribers {
		if s.id != subID {
			filtered = append(filtered, s)
		}
	}
	cg.subscribers = filtered
}

func (cg *consumerGroup) deliver(msg *Message) bool {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	if len(cg.subscribers) == 0 {
		return false
	}

	idx := atomic.AddUint64(&cg.roundRobin, 1) % uint64(len(cg.subscribers))
	target := cg.subscribers[idx]

	deliverToSub(target, msg)
	return true
}

func deliverToSub(sub *Subscription, msg *Message) {
	if sub.closed.Load() {
		return
	}

	switch sub.strategy {
	case BlockOnFull:
		sub.ch <- msg
	case DropNewest:
		select {
		case sub.ch <- msg:
		default:
			// Dropped newest
		}
	case DropOldest:
		select {
		case sub.ch <- msg:
		default:
			select {
			case <-sub.ch:
			default:
			}
			select {
			case sub.ch <- msg:
			default:
			}
		}
	}
}

// Engine manages topic routing, pattern matching, consumer groups, and deduplication.
type Engine struct {
	mu             sync.RWMutex
	exactSubs      map[string][]*Subscription
	groupSubs      map[string]map[string]*consumerGroup // topic -> groupName -> group
	patternSubs    []*patternSub
	dedup          *queue.Deduplicator
	subSeq         atomic.Uint64
	msgSeq         atomic.Uint64
	totalPublished atomic.Uint64
	totalDelivered atomic.Uint64
	defaultBufSize int
}

// Stats returns real-time broker statistics.
type Stats struct {
	TotalPublished uint64
	TotalDelivered uint64
	ActiveTopics   int
	ActiveSubs     int
}

func (e *Engine) Stats() Stats {
	e.mu.RLock()
	activeTopics := len(e.exactSubs) + len(e.groupSubs)
	activeSubs := len(e.patternSubs)
	for _, subs := range e.exactSubs {
		activeSubs += len(subs)
	}
	for _, groups := range e.groupSubs {
		for _, cg := range groups {
			cg.mu.Lock()
			activeSubs += len(cg.subscribers)
			cg.mu.Unlock()
		}
	}
	e.mu.RUnlock()

	return Stats{
		TotalPublished: e.totalPublished.Load(),
		TotalDelivered: e.totalDelivered.Load(),
		ActiveTopics:   activeTopics,
		ActiveSubs:     activeSubs,
	}
}

type patternSub struct {
	pattern string
	sub     *Subscription
}

func NewEngine(defaultBufSize int) *Engine {
	if defaultBufSize <= 0 {
		defaultBufSize = 256
	}
	return &Engine{
		exactSubs:      make(map[string][]*Subscription),
		groupSubs:      make(map[string]map[string]*consumerGroup),
		patternSubs:    make([]*patternSub, 0),
		dedup:          queue.NewDeduplicator(),
		defaultBufSize: defaultBufSize,
	}
}

// Subscribe subscribes to a topic or wildcard pattern with direct broadcast.
func (e *Engine) Subscribe(ctx context.Context, topicPattern string, bufSize int, strategy BackpressureStrategy) *Subscription {
	if bufSize <= 0 {
		bufSize = e.defaultBufSize
	}

	subID := e.subSeq.Add(1)
	sub := &Subscription{
		id:       subID,
		topic:    topicPattern,
		ch:       make(chan *Message, bufSize),
		strategy: strategy,
	}

	isPattern := strings.ContainsAny(topicPattern, "*>")

	e.mu.Lock()
	defer e.mu.Unlock()

	if isPattern {
		e.patternSubs = append(e.patternSubs, &patternSub{pattern: topicPattern, sub: sub})
		sub.unsubFn = func() {
			e.mu.Lock()
			defer e.mu.Unlock()
			filtered := e.patternSubs[:0]
			for _, ps := range e.patternSubs {
				if ps.sub.id != subID {
					filtered = append(filtered, ps)
				}
			}
			e.patternSubs = filtered
		}
	} else {
		e.exactSubs[topicPattern] = append(e.exactSubs[topicPattern], sub)
		sub.unsubFn = func() {
			e.mu.Lock()
			defer e.mu.Unlock()
			subs := e.exactSubs[topicPattern]
			filtered := subs[:0]
			for _, s := range subs {
				if s.id != subID {
					filtered = append(filtered, s)
				}
			}
			if len(filtered) == 0 {
				delete(e.exactSubs, topicPattern)
			} else {
				e.exactSubs[topicPattern] = filtered
			}
		}
	}

	return sub
}

// SubscribeGroup subscribes to a topic under a Consumer Group name for load-shared competing consumers.
func (e *Engine) SubscribeGroup(ctx context.Context, topic string, groupName string, bufSize int, strategy BackpressureStrategy) *Subscription {
	if bufSize <= 0 {
		bufSize = e.defaultBufSize
	}

	subID := e.subSeq.Add(1)
	sub := &Subscription{
		id:       subID,
		topic:    topic,
		group:    groupName,
		ch:       make(chan *Message, bufSize),
		strategy: strategy,
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	groups, ok := e.groupSubs[topic]
	if !ok {
		groups = make(map[string]*consumerGroup)
		e.groupSubs[topic] = groups
	}

	cg, ok := groups[groupName]
	if !ok {
		cg = &consumerGroup{name: groupName}
		groups[groupName] = cg
	}
	cg.add(sub)

	sub.unsubFn = func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		if groups, ok := e.groupSubs[topic]; ok {
			if cg, ok := groups[groupName]; ok {
				cg.remove(subID)
			}
		}
	}

	return sub
}

// Publish broadcasts a message to all matching subscribers, honoring deduplication windows.
func (e *Engine) Publish(topic string, payload []byte, dedupID string, dedupWindow time.Duration) (int, error) {
	if dedupID != "" {
		if !e.dedup.CheckOrSet(dedupID, dedupWindow) {
			return 0, queue.ErrDuplicateMessage
		}
	}

	now := time.Now().UnixNano()
	seq := e.msgSeq.Add(1)
	e.totalPublished.Add(1)
	msg := &Message{
		ID:        strings.Join([]string{topic, string(rune(seq))}, "_"),
		Topic:     topic,
		Payload:   payload,
		DedupID:   dedupID,
		CreatedAt: now,
	}

	e.mu.RLock()
	defer e.mu.RUnlock()

	deliveredCount := 0

	// 1. Direct exact subscribers (broadcast)
	if subs, ok := e.exactSubs[topic]; ok {
		for _, sub := range subs {
			deliverToSub(sub, msg)
			deliveredCount++
		}
	}

	// 2. Consumer group subscribers (1 delivery per group)
	if groups, ok := e.groupSubs[topic]; ok {
		for _, cg := range groups {
			if cg.deliver(msg) {
				deliveredCount++
			}
		}
	}

	// 3. Pattern subscribers (wildcards)
	for _, ps := range e.patternSubs {
		if matchTopic(ps.pattern, topic) {
			deliverToSub(ps.sub, msg)
			deliveredCount++
		}
	}

	e.totalDelivered.Add(uint64(deliveredCount))
	return deliveredCount, nil
}

// matchTopic checks wildcard matching where '.' is delimiter, '*' matches 1 segment, '>' matches 1 or more segments.
func matchTopic(pattern, topic string) bool {
	if pattern == topic || pattern == ">" {
		return true
	}

	pParts := strings.Split(pattern, ".")
	tParts := strings.Split(topic, ".")

	pLen := len(pParts)
	tLen := len(tParts)

	for i := 0; i < pLen; i++ {
		p := pParts[i]
		if p == ">" {
			// Matches rest of topic
			return true
		}
		if i >= tLen {
			return false
		}
		if p != "*" && p != tParts[i] {
			return false
		}
	}

	return pLen == tLen
}
