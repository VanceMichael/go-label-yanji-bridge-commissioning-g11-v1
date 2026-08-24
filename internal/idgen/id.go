package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
)

func New(prefix string) string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		panic(fmt.Sprintf("secure random source unavailable: %v", err))
	}
	prefix = strings.Trim(strings.ToLower(prefix), "-_")
	return prefix + "_" + hex.EncodeToString(buffer)
}
