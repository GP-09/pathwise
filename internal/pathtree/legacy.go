package pathtree

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// This state belongs to one parsed document, not to rules or conditional paths.
// Only object key indexes and decoded intermediate documents are retained.
type legacyNode struct {
	value      any
	keysOnce   sync.Once
	canonical  bool
	folded     map[string]string
	ambiguous  map[string]bool
	children   sync.Map
	decodeOnce sync.Once
	decoded    *legacyNode
}

func (n *legacyNode) index(value map[string]any) {
	n.keysOnce.Do(func() {
		n.canonical = true
		for key := range value {
			if key != strings.ToLower(key) {
				n.canonical = false
				break
			}
		}
		if n.canonical {
			return
		}
		n.folded = make(map[string]string, len(value))
		for key := range value {
			lower := strings.ToLower(key)
			if _, exists := n.folded[lower]; exists {
				if n.ambiguous == nil {
					n.ambiguous = map[string]bool{}
				}
				n.ambiguous[lower] = true
			}
			n.folded[lower] = key
		}
	})
}

func (n *legacyNode) child(key, value any) *legacyNode {
	if found, ok := n.children.Load(key); ok {
		return found.(*legacyNode)
	}
	found, _ := n.children.LoadOrStore(key, &legacyNode{value: value})
	return found.(*legacyNode)
}

func (n *legacyNode) resolve(keys []string, onError func(error)) (any, bool) {
	switch value := n.value.(type) {
	case map[string]any:
		n.index(value)
		for count := len(keys); count > 0; count-- {
			key := strings.ToLower(strings.Join(keys[:count], "."))
			original := key
			found := false
			if n.canonical {
				_, found = value[key]
			} else {
				original, found = n.folded[key]
				if n.ambiguous[key] {
					for candidate := range value {
						if strings.ToLower(candidate) == key {
							original = candidate
							break
						}
					}
				}
			}
			if !found {
				continue
			}
			child := value[original]
			if count == len(keys) {
				return child, true
			}
			switch child.(type) {
			case map[string]any, []any, string, []byte, json.RawMessage:
				return n.child(original, child).resolve(keys[count:], onError)
			default:
				return nil, false
			}
		}
		return nil, false
	case []any:
		index, err := strconv.Atoi(keys[0])
		if err != nil {
			if onError != nil {
				onError(err)
			}
			return nil, false
		}
		if index < 0 || index >= len(value) {
			return nil, false
		}
		if len(keys) == 1 {
			return value[index], true
		}
		return n.child(index, value[index]).resolve(keys[1:], onError)
	case string, []byte, json.RawMessage:
		var text string
		switch value := value.(type) {
		case string:
			text = value
		case []byte:
			text = string(value)
		case json.RawMessage:
			text = string(value)
		}
		if decoded, err := url.QueryUnescape(text); err == nil {
			text = decoded
		}
		n.decodeOnce.Do(func() {
			if decoded, ok := structured(text); ok {
				n.decoded = &legacyNode{value: decoded}
			}
		})
		if n.decoded != nil {
			return n.decoded.resolve(keys, onError)
		}
		return strings.ToLower(text), true
	default:
		return value, true
	}
}

func structured(text string) (any, bool) {
	if value, ok := structuredBytes([]byte(text)); ok {
		return value, true
	}
	if decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text)); err == nil {
		return structuredBytes(decoded)
	}
	return nil, false
}

func structuredBytes(raw []byte) (any, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || (raw[0] != '{' && raw[0] != '[') {
		return nil, false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return nil, false
	}
	switch value.(type) {
	case map[string]any, []any:
		return value, true
	}
	return nil, false
}
