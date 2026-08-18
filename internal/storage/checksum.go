package storage

import (
	"hash/crc32"
)

var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

// Checksum computes the CRC32C checksum of data.
func Checksum(data []byte) uint32 {
	return crc32.Checksum(data, crc32cTable)
}

// UpdateChecksum updates an existing CRC32C checksum.
func UpdateChecksum(crc uint32, data []byte) uint32 {
	return crc32.Update(crc, crc32cTable, data)
}
