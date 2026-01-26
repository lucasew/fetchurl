package maxsize

// Policy triggers eviction when cache exceeds a fixed size.
type Policy struct {
	MaxBytes int64
}

func (m *Policy) BytesToFree(currentSize int64) (int64, error) {
	if currentSize > m.MaxBytes {
		return currentSize - m.MaxBytes, nil
	}
	return 0, nil
}
