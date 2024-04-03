package pkg

import (
	"math/rand"
	"time"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func GenRandomSuffix() string {
	r := rand.New(rand.NewSource(time.Now().Unix())) // #nosec G404
	suffix := make([]byte, 6)
	for i := range suffix {
		suffix[i] = charset[r.Intn(len(charset))]
	}

	return string(suffix)
}
