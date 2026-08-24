package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonMemory      = 19 * 1024
	argonIterations  = 2
	argonParallelism = 1
	argonSaltBytes   = 16
	argonOutputBytes = 32
	minimumPassword  = 12
	maximumPassword  = 1024
)

var dummyPasswordHash = mustDummyPasswordHash()

func HashPassword(password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, argonSaltBytes)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonOutputBytes)
	return encodePasswordHash(salt, hash), nil
}

func VerifyPassword(encoded string, password string) bool {
	if len(password) > maximumPassword {
		return false
	}
	parameters, salt, expected, err := decodePasswordHash(encoded)
	if err != nil {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, parameters.iterations, parameters.memory, parameters.parallelism, uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

func VerifyUnknownPassword(password string) {
	VerifyPassword(dummyPasswordHash, password)
}

func validatePassword(password string) error {
	if len(password) < minimumPassword {
		return fmt.Errorf("%w: password must contain at least %d bytes", ErrPasswordTooShort, minimumPassword)
	}
	if len(password) > maximumPassword {
		return fmt.Errorf("%w: password must contain at most %d bytes", ErrPasswordTooLong, maximumPassword)
	}
	return nil
}

func encodePasswordHash(salt []byte, hash []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemory, argonIterations, argonParallelism, base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash))
}

func decodePasswordHash(encoded string) (passwordParameters, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return passwordParameters{}, nil, nil, errors.New("invalid password encoding")
	}
	parameters, err := parseParameters(parts[3])
	if err != nil || parameters != configuredPasswordParameters() {
		return passwordParameters{}, nil, nil, errors.New("unsupported password parameters")
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != argonSaltBytes {
		return passwordParameters{}, nil, nil, errors.New("invalid password salt")
	}
	hash, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(hash) != argonOutputBytes {
		return passwordParameters{}, nil, nil, errors.New("invalid password hash")
	}
	return parameters, salt, hash, nil
}

func parseParameters(value string) (passwordParameters, error) {
	var result passwordParameters
	parts := strings.Split(value, ",")
	if len(parts) != 3 {
		return result, errors.New("invalid parameters")
	}
	memory, err := strconv.ParseUint(strings.TrimPrefix(parts[0], "m="), 10, 32)
	if err != nil {
		return result, err
	}
	iterations, err := strconv.ParseUint(strings.TrimPrefix(parts[1], "t="), 10, 32)
	if err != nil {
		return result, err
	}
	parallelism, err := strconv.ParseUint(strings.TrimPrefix(parts[2], "p="), 10, 8)
	if err != nil {
		return result, err
	}
	return passwordParameters{memory: uint32(memory), iterations: uint32(iterations), parallelism: uint8(parallelism)}, nil
}

func configuredPasswordParameters() passwordParameters {
	return passwordParameters{memory: argonMemory, iterations: argonIterations, parallelism: argonParallelism}
}

func mustDummyPasswordHash() string {
	salt := make([]byte, argonSaltBytes)
	hash := argon2.IDKey([]byte("not-a-user-password"), salt, argonIterations, argonMemory, argonParallelism, argonOutputBytes)
	return encodePasswordHash(salt, hash)
}

type passwordParameters struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}
