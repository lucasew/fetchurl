package hashutil

import (
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"fmt"
	"hash"
)

type HashFactory func() hash.Hash

var registry = map[string]HashFactory{
	"sha1":   sha1.New,
	"sha256": sha256.New,
	"sha512": sha512.New,
}

func Register(name string, factory HashFactory) {
	registry[name] = factory
}

func GetHasher(name string) (hash.Hash, error) {
	factory, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unsupported hash algorithm: %s", name)
	}
	return factory(), nil
}

func IsSupported(name string) bool {
	_, ok := registry[name]
	return ok
}
