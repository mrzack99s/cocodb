package queue

import (
	"encoding/binary"
	"errors"
	"fmt"
	"time"
)

var (
	ErrDuplicateMessage   = errors.New("coco/queue: duplicate message detected within deduplication window")
	ErrQueueEmpty         = errors.New("coco/queue: queue is empty")
	ErrMessageNotFound    = errors.New("coco/queue: message not found")
	ErrMessageExpired     = errors.New("coco/queue: message expired")
	ErrMaxRetriesExceeded = errors.New("coco/queue: max retries exceeded, routed to DLQ")
)

type MessageState uint8

const (
	StateReady MessageState = iota
	StateInvisible
	StateCompleted
	StateDeadLetter
)

func (s MessageState) String() string {
	switch s {
	case StateReady:
		return "ready"
	case StateInvisible:
		return "invisible"
	case StateCompleted:
		return "completed"
	case StateDeadLetter:
		return "dead_letter"
	default:
		return fmt.Sprintf("state_%d", s)
	}
}

// Message represents a task or payload in the queue.
type Message struct {
	ID         string
	Queue      string
	Payload    []byte
	Priority   uint8
	DedupID    string
	RetryCount int
	MaxRetries int
	State      MessageState
	VisibleAt  int64 // UnixNano
	CreatedAt  int64 // UnixNano
	ExpireAt   int64 // UnixNano (0 = no expiration)

	// Callback handlers
	ackFn    func() error
	nackFn   func(requeue bool) error
	extendFn func(duration time.Duration) error
}

// Ack marks the message as successfully processed and removes it from the queue.
func (m *Message) Ack() error {
	if m.ackFn != nil {
		return m.ackFn()
	}
	return nil
}

// Nack rejects the message. If requeue is true, it becomes immediately visible for another worker.
func (m *Message) Nack(requeue bool) error {
	if m.nackFn != nil {
		return m.nackFn(requeue)
	}
	return nil
}

// Extend extends the visibility timeout lease for this in-flight message.
func (m *Message) Extend(d time.Duration) error {
	if m.extendFn != nil {
		return m.extendFn(d)
	}
	return nil
}

// SetCallbacks binds runtime lifecycle callbacks to the message.
func (m *Message) SetCallbacks(ack func() error, nack func(bool) error, extend func(time.Duration) error) {
	m.ackFn = ack
	m.nackFn = nack
	m.extendFn = extend
}

// Encode serializes the Message into a compact binary format.
func (m *Message) Encode() []byte {
	idBytes := []byte(m.ID)
	qBytes := []byte(m.Queue)
	dedupBytes := []byte(m.DedupID)

	fixedSize := 2 + 1 + 1 + 4 + 4 + 8 + 8 + 8 + 2 + len(idBytes) + 2 + len(qBytes) + 2 + len(dedupBytes) + 4 + len(m.Payload)
	buf := make([]byte, fixedSize)

	buf[0] = 'Q'
	buf[1] = 'M'
	buf[2] = byte(m.State)
	buf[3] = m.Priority
	binary.BigEndian.PutUint32(buf[4:8], uint32(m.RetryCount))
	binary.BigEndian.PutUint32(buf[8:12], uint32(m.MaxRetries))
	binary.BigEndian.PutUint64(buf[12:20], uint64(m.VisibleAt))
	binary.BigEndian.PutUint64(buf[20:28], uint64(m.CreatedAt))
	binary.BigEndian.PutUint64(buf[28:36], uint64(m.ExpireAt))

	offset := 36
	binary.BigEndian.PutUint16(buf[offset:offset+2], uint16(len(idBytes)))
	offset += 2
	copy(buf[offset:], idBytes)
	offset += len(idBytes)

	binary.BigEndian.PutUint16(buf[offset:offset+2], uint16(len(qBytes)))
	offset += 2
	copy(buf[offset:], qBytes)
	offset += len(qBytes)

	binary.BigEndian.PutUint16(buf[offset:offset+2], uint16(len(dedupBytes)))
	offset += 2
	copy(buf[offset:], dedupBytes)
	offset += len(dedupBytes)

	binary.BigEndian.PutUint32(buf[offset:offset+4], uint32(len(m.Payload)))
	offset += 4
	copy(buf[offset:], m.Payload)

	return buf
}

// DecodeMessage deserializes raw bytes into a Message struct.
func DecodeMessage(buf []byte) (*Message, error) {
	if len(buf) < 36 {
		return nil, errors.New("coco/queue: buffer too small for message header")
	}
	if buf[0] != 'Q' || buf[1] != 'M' {
		return nil, errors.New("coco/queue: invalid message magic")
	}

	m := &Message{
		State:      MessageState(buf[2]),
		Priority:   buf[3],
		RetryCount: int(binary.BigEndian.Uint32(buf[4:8])),
		MaxRetries: int(binary.BigEndian.Uint32(buf[8:12])),
		VisibleAt:  int64(binary.BigEndian.Uint64(buf[12:20])),
		CreatedAt:  int64(binary.BigEndian.Uint64(buf[20:28])),
		ExpireAt:   int64(binary.BigEndian.Uint64(buf[28:36])),
	}

	offset := 36
	if len(buf) < offset+2 {
		return nil, errors.New("coco/queue: truncated ID length")
	}
	idLen := int(binary.BigEndian.Uint16(buf[offset : offset+2]))
	offset += 2
	if len(buf) < offset+idLen {
		return nil, errors.New("coco/queue: truncated ID data")
	}
	m.ID = string(buf[offset : offset+idLen])
	offset += idLen

	if len(buf) < offset+2 {
		return nil, errors.New("coco/queue: truncated Queue length")
	}
	qLen := int(binary.BigEndian.Uint16(buf[offset : offset+2]))
	offset += 2
	if len(buf) < offset+qLen {
		return nil, errors.New("coco/queue: truncated Queue data")
	}
	m.Queue = string(buf[offset : offset+qLen])
	offset += qLen

	if len(buf) < offset+2 {
		return nil, errors.New("coco/queue: truncated DedupID length")
	}
	dedupLen := int(binary.BigEndian.Uint16(buf[offset : offset+2]))
	offset += 2
	if len(buf) < offset+dedupLen {
		return nil, errors.New("coco/queue: truncated DedupID data")
	}
	m.DedupID = string(buf[offset : offset+dedupLen])
	offset += dedupLen

	if len(buf) < offset+4 {
		return nil, errors.New("coco/queue: truncated Payload length")
	}
	payloadLen := int(binary.BigEndian.Uint32(buf[offset : offset+4]))
	offset += 4
	if len(buf) < offset+payloadLen {
		return nil, errors.New("coco/queue: truncated Payload data")
	}
	m.Payload = make([]byte, payloadLen)
	copy(m.Payload, buf[offset:offset+payloadLen])

	return m, nil
}
