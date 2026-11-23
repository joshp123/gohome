package roborock

import (
	"fmt"
	"hash/crc32"
	"math/rand"
	"time"
)

func nowTimestamp() uint32 {
	return uint32(time.Now().Unix())
}

func nextInt(min, max int) int {
	if max <= min {
		return min
	}
	return rand.Intn(max-min) + min
}

func encodeTimestamp(ts uint32) []byte {
	hex := fmt.Sprintf("%08x", ts)
	order := []int{5, 6, 3, 7, 1, 2, 0, 4}
	out := make([]byte, 8)
	for i, idx := range order {
		out[i] = hex[idx]
	}
	return out
}

func crc32sum(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}
