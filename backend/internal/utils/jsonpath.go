package utils

import (
	"strconv"
	"strings"
)

// ExtractValue implements a simple JSONPath-like extractor
// Supports: $.key, $.nested.key, $.key[:4]
func ExtractValue(data interface{}, path string) interface{} {
	path = strings.TrimPrefix(path, "$.")

	// Handle slice syntax at the end
	var sliceEnd int = -1
	if idx := strings.Index(path, "[:"); idx != -1 && strings.HasSuffix(path, "]") {
		sliceStr := path[idx+2 : len(path)-1]
		if i, err := strconv.Atoi(sliceStr); err == nil {
			sliceEnd = i
		}
		path = path[:idx]
	}

	parts := strings.Split(path, ".")
	current := data

	for _, part := range parts {
		// Check for array index: key[0]
		var index int = -1
		key := part
		if idx := strings.Index(part, "["); idx != -1 && strings.HasSuffix(part, "]") {
			idxStr := part[idx+1 : len(part)-1]
			if i, err := strconv.Atoi(idxStr); err == nil {
				index = i
				key = part[:idx]
			}
		}

		// Navigate map
		if key != "" {
			m, ok := current.(map[string]interface{})
			if !ok {
				return nil
			}
			val, exists := m[key]
			if !exists {
				return nil
			}
			current = val
		}

		// Navigate array if index was present
		if index != -1 {
			arr, ok := current.([]interface{})
			if !ok {
				return nil
			}
			if index < 0 || index >= len(arr) {
				return nil
			}
			current = arr[index]
		}
	}

	if sliceEnd != -1 {
		if str, ok := current.(string); ok {
			if len(str) > sliceEnd {
				return str[:sliceEnd]
			}
			return str
		}
	}

	return current
}