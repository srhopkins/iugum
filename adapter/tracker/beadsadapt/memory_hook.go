package beadsadapt

import (
	"context"

	bdcmd "github.com/steveyegge/beads/cmd/bd"

	"github.com/srhopkins/iugum/contract"
)

const beadsMemNS = "default"
const beadsMemType = "fact"

var sqliteHook bdcmd.MemoryHook

// UseMemory wraps contract.Memory as the bd MemoryHook (ns default, type fact).
func UseMemory(m contract.Memory) {
	sqliteHook = &memoryHook{mem: m}
}

type memoryHook struct {
	mem contract.Memory
}

func (h *memoryHook) Remember(key, value string) error {
	return h.mem.Remember(context.Background(), contract.MemoryRec{
		NS: beadsMemNS, Type: beadsMemType, Key: key, Value: value,
	})
}

func (h *memoryHook) Recall(key string) (string, bool, error) {
	rec, ok, err := h.mem.Recall(context.Background(), beadsMemNS, key)
	if err != nil || !ok {
		return "", ok, err
	}
	return rec.Value, true, nil
}

func (h *memoryHook) Forget(key string) error {
	return h.mem.Forget(context.Background(), beadsMemNS, key)
}

func (h *memoryHook) List(query string) (map[string]string, error) {
	hits, err := h.mem.Search(context.Background(), contract.MemoryQuery{
		NS:    []string{beadsMemNS},
		Type:  beadsMemType,
		Text:  query,
		Limit: 10000,
	})
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(hits))
	for _, hit := range hits {
		out[hit.Key] = hit.Value
	}
	return out, nil
}
