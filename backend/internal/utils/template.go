package utils

import (
	"fmt"
	"strings"
)

// ResolveTemplate resolves a string template with placeholders {Key} or {Key:Format}
// Data is a map of values. Supports dot notation for nested keys.
func ResolveTemplate(template string, data map[string]interface{}) (string, error) {
	result := template
	start := 0
	for {
		startIdx := strings.Index(result[start:], "{")
		if startIdx == -1 {
			break
		}
		startIdx += start

		endIdx := strings.Index(result[startIdx:], "}")
		if endIdx == -1 {
			break
		}
		endIdx += startIdx

		fullPlaceholder := result[startIdx : endIdx+1] // e.g. "{Series.Title}"
		content := result[startIdx+1 : endIdx]         // e.g. "Series.Title"

		// Parse content for format
		parts := strings.Split(content, ":")
		keyPath := parts[0]
		format := ""
		if len(parts) > 1 {
			format = parts[1]
		}

		// Resolve value
		val := ExtractValue(data, keyPath)
		if val != nil {
			replacement := fmt.Sprintf("%v", val)

			// Apply format if needed
			if format == "02d" {
				if i, ok := toInt(val); ok {
					replacement = fmt.Sprintf("%02d", i)
				}
			}

			result = strings.Replace(result, fullPlaceholder, replacement, 1)
			// Don't advance start, scan again in case we replaced it with something containing braces (unlikely but safe)
			// or simply to find the next one.
			// Since we replaced one instance, the indices shifted.
			// Ideally we should just restart or handle it carefully.
			// Since we act on the first match, restarting from the same start index (or 0) is safe but inefficient.
			// However, since we used strings.Replace with n=1, we replaced the *first* occurrence.
			// So if we continue from startIdx, we might skip if the replacement was shorter?
			// No, simply continuing the loop will find the next `{`.
			// But since `result` changed, `start` might need adjustment.
			// Actually, if we replaced {Foo} with Bar, the length changed.
			// Safe bet: reset start to startIdx (where the replacement started).
			start = startIdx
		} else {
			// Value not found, skip this placeholder to avoid infinite loop
			start = endIdx + 1
		}
	}

	return result, nil
}

func toInt(v interface{}) (int, bool) {
	switch i := v.(type) {
	case int:
		return i, true
	case float64:
		return int(i), true
	default:
		return 0, false
	}
}
