package cson

import (
	"bytes"
	"encoding/binary"
	"sync"
)

type ValueType uint8

const (
	TypeNull ValueType = iota
	TypeBool
	TypeInt64
	TypeUint64
	TypeFloat64
	TypeString
	TypeBytes
	TypeTime
	TypeArray
	TypeObject
	TypeVectorFloat32
)

const (
	CSONMagic   uint32 = 0x43534F4E // "CSON"
	CSONVersion uint16 = 1

	FieldEntrySize = 14 // 4 (FieldID) + 1 (Type) + 1 (res) + 4 (Offset) + 4 (Length)
)

// FieldDictionary maps field names to compact numeric FieldIDs.
type FieldDictionary struct {
	mu       sync.RWMutex
	nameToID map[string]uint32
	idToName map[uint32]string
	nextID   uint32
}

func NewFieldDictionary() *FieldDictionary {
	d := &FieldDictionary{
		nameToID: make(map[string]uint32),
		idToName: make(map[uint32]string),
		nextID:   1,
	}
	// Reserve standard field "_id" as FieldID 1
	d.nameToID["_id"] = 1
	d.idToName[1] = "_id"
	d.nextID = 2
	return d
}

// GetOrAssignID returns the existing FieldID or assigns a new one.
func (d *FieldDictionary) GetOrAssignID(name string) uint32 {
	d.mu.RLock()
	if id, ok := d.nameToID[name]; ok {
		d.mu.RUnlock()
		return id
	}
	d.mu.RUnlock()

	d.mu.Lock()
	defer d.mu.Unlock()

	if id, ok := d.nameToID[name]; ok {
		return id
	}
	id := d.nextID
	d.nextID++
	d.nameToID[name] = id
	d.idToName[id] = name
	return id
}

// GetID returns the FieldID for a name if registered.
func (d *FieldDictionary) GetID(name string) (uint32, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	id, ok := d.nameToID[name]
	return id, ok
}

// GetName returns the name for a FieldID.
func (d *FieldDictionary) GetName(id uint32) (string, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	name, ok := d.idToName[id]
	return name, ok
}

// Encode serializes the field dictionary into a binary byte slice.
func (d *FieldDictionary) Encode() []byte {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.BigEndian, d.nextID)
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(d.nameToID)))

	for name, id := range d.nameToID {
		_ = binary.Write(&buf, binary.BigEndian, id)
		_ = binary.Write(&buf, binary.BigEndian, uint16(len(name)))
		buf.WriteString(name)
	}

	return buf.Bytes()
}

// Decode deserializes the field dictionary.
func (d *FieldDictionary) Decode(data []byte) error {
	if len(data) < 8 {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	r := bytes.NewReader(data)
	if err := binary.Read(r, binary.BigEndian, &d.nextID); err != nil {
		return err
	}
	var count uint32
	if err := binary.Read(r, binary.BigEndian, &count); err != nil {
		return err
	}

	for i := uint32(0); i < count; i++ {
		var id uint32
		var nameLen uint16
		if err := binary.Read(r, binary.BigEndian, &id); err != nil {
			return err
		}
		if err := binary.Read(r, binary.BigEndian, &nameLen); err != nil {
			return err
		}
		nameBuf := make([]byte, nameLen)
		if _, err := r.Read(nameBuf); err != nil {
			return err
		}
		name := string(nameBuf)
		d.nameToID[name] = id
		d.idToName[id] = name
	}

	return nil
}
