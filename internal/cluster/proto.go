package cluster

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"sync"
)

var (
	ErrInvalidMagic       = errors.New("coco/cluster: invalid magic bytes in header")
	ErrUnsupportedVersion = errors.New("coco/cluster: unsupported protocol version")
	ErrChecksumMismatch   = errors.New("coco/cluster: frame CRC32C checksum mismatch")
	ErrPayloadTooLarge    = errors.New("coco/cluster: payload exceeds maximum frame size (64MB)")
	ErrFrameTruncated     = errors.New("coco/cluster: truncated network frame")
	ErrUnauthorized       = errors.New("coco/cluster: unauthorized, invalid credentials or token")
	ErrPeerUnavailable    = errors.New("coco/cluster: peer cluster node unavailable")
)

const (
	ProtocolMagic   = 0x4F434F43 // 'COCO' in BigEndian
	ProtocolVersion = 1
	HeaderSize      = 16
	MaxPayloadSize  = 64 * 1024 * 1024 // 64 MB
)

// Command IDs
const (
	CmdAuth      uint8 = 0x01
	CmdPing      uint8 = 0x02
	CmdEnqueue   uint8 = 0x10
	CmdDequeue   uint8 = 0x11
	CmdAck       uint8 = 0x12
	CmdNack      uint8 = 0x13
	CmdPublish   uint8 = 0x20
	CmdSubscribe uint8 = 0x21
	CmdForward   uint8 = 0x30
	CmdResponse  uint8 = 0x80
)

// Status / Return codes
const (
	StatusOK           uint8 = 0x00
	StatusError        uint8 = 0x01
	StatusUnauthorized uint8 = 0x02
	StatusDuplicate    uint8 = 0x03
	StatusNotFound     uint8 = 0x04
	StatusQueueEmpty   uint8 = 0x05
)

// Frame represents a raw wire-level RPC packet.
type Frame struct {
	Cmd           uint8
	Status        uint8
	CorrelationID uint32
	Payload       []byte
}

var (
	crcTable   = crc32.MakeTable(crc32.Castagnoli)
	headerPool = sync.Pool{
		New: func() any {
			b := make([]byte, HeaderSize)
			return &b
		},
	}
)

// EncodeFrame writes a Frame to an io.Writer with 16-byte header + payload + 4-byte CRC32C.
func EncodeFrame(w io.Writer, f *Frame) error {
	hBuf := headerPool.Get().(*[]byte)
	buf := *hBuf
	defer headerPool.Put(hBuf)

	binary.BigEndian.PutUint32(buf[0:4], ProtocolMagic)
	buf[4] = ProtocolVersion
	buf[5] = f.Cmd
	buf[6] = f.Status
	buf[7] = 0 // Reserved
	binary.BigEndian.PutUint32(buf[8:12], f.CorrelationID)
	binary.BigEndian.PutUint32(buf[12:16], uint32(len(f.Payload)))

	if _, err := w.Write(buf); err != nil {
		return err
	}

	if len(f.Payload) > 0 {
		if _, err := w.Write(f.Payload); err != nil {
			return err
		}
		var crcBuf [4]byte
		checksum := crc32.Checksum(f.Payload, crcTable)
		binary.BigEndian.PutUint32(crcBuf[:], checksum)
		if _, err := w.Write(crcBuf[:]); err != nil {
			return err
		}
	}

	return nil
}

// DecodeFrame reads a Frame from an io.Reader, verifying magic, version, and CRC32C.
func DecodeFrame(r io.Reader) (*Frame, error) {
	hBuf := headerPool.Get().(*[]byte)
	buf := *hBuf
	defer headerPool.Put(hBuf)

	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	magic := binary.BigEndian.Uint32(buf[0:4])
	if magic != ProtocolMagic {
		return nil, ErrInvalidMagic
	}

	ver := buf[4]
	if ver != ProtocolVersion {
		return nil, ErrUnsupportedVersion
	}

	cmd := buf[5]
	status := buf[6]
	corrID := binary.BigEndian.Uint32(buf[8:12])
	payloadLen := binary.BigEndian.Uint32(buf[12:16])

	if payloadLen > MaxPayloadSize {
		return nil, ErrPayloadTooLarge
	}

	var payload []byte
	if payloadLen > 0 {
		payload = make([]byte, payloadLen)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, err
		}

		var crcBuf [4]byte
		if _, err := io.ReadFull(r, crcBuf[:]); err != nil {
			return nil, err
		}
		expectedCRC := binary.BigEndian.Uint32(crcBuf[:])
		actualCRC := crc32.Checksum(payload, crcTable)
		if expectedCRC != actualCRC {
			return nil, ErrChecksumMismatch
		}
	}

	return &Frame{
		Cmd:           cmd,
		Status:        status,
		CorrelationID: corrID,
		Payload:       payload,
	}, nil
}
