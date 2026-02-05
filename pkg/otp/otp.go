package otp

import (
	"crypto/rand"
	"math/big"
)

func Generate(length int) (string, error) {
	const digits = "0123456789"
	otp := make([]byte, length)
	for i := range otp {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		otp[i] = digits[n.Int64()]
	}
	return string(otp), nil
}
