package hostdb

import (
	"testing"
)

// memPersist implements the persister interface in-memory.
type memPersist hdbPersist

func (m *memPersist) save(data hdbPersist) error { *m = memPersist(data); return nil }
func (m memPersist) load(data *hdbPersist) error { *data = hdbPersist(m); return nil }

// TestSaveLoad tests that the hostdb can save and load itself.
func TestSaveLoad(t *testing.T) {
	t.Skip("host persistence not implemented")
}
