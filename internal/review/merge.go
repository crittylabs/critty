package review

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"
)

func mergeReview(base []byte, local, fresh CritJSON) (CritJSON, error) {
	if base == nil {
		var err error
		base, err = json.Marshal(NewCritJSON())
		if err != nil {
			return CritJSON{}, err
		}
	}
	values := make([]any, 3)
	for i, state := range []any{json.RawMessage(base), local, fresh} {
		data, err := json.Marshal(state)
		if err != nil {
			return CritJSON{}, err
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&values[i]); err != nil {
			return CritJSON{}, err
		}
	}
	merged, err := mergeValue(values[0], values[1], values[2], "review")
	if err != nil {
		return CritJSON{}, err
	}
	data, err := json.Marshal(merged)
	if err != nil {
		return CritJSON{}, err
	}
	var result CritJSON
	if err := json.Unmarshal(data, &result); err != nil {
		return CritJSON{}, err
	}
	return result, nil
}

func mergeValue(base, local, fresh any, path string) (any, error) {
	switch {
	case reflect.DeepEqual(local, base):
		return fresh, nil
	case reflect.DeepEqual(fresh, base), reflect.DeepEqual(local, fresh):
		return local, nil
	}
	if strings.HasSuffix(path, ".updated_at") {
		localTime, localErr := time.Parse(time.RFC3339Nano, fmt.Sprint(local))
		freshTime, freshErr := time.Parse(time.RFC3339Nano, fmt.Sprint(fresh))
		if localErr == nil && freshErr == nil {
			if freshTime.After(localTime) {
				return fresh, nil
			}
			return local, nil
		}
	}
	if localMap, ok := local.(map[string]any); ok {
		if freshMap, ok := fresh.(map[string]any); ok {
			baseMap, _ := base.(map[string]any)
			keys := map[string]bool{}
			for _, object := range []map[string]any{baseMap, localMap, freshMap} {
				for key := range object {
					keys[key] = true
				}
			}
			merged := map[string]any{}
			for key := range keys {
				value, err := mergeValue(baseMap[key], localMap[key], freshMap[key], path+"."+key)
				if err != nil {
					return nil, err
				}
				if value != nil {
					merged[key] = value
				}
			}
			return merged, nil
		}
	}
	if strings.HasSuffix(path, ".comments") || strings.HasSuffix(path, ".review_comments") || strings.HasSuffix(path, ".replies") {
		return mergeEntries(base, local, fresh, path)
	}
	return nil, fmt.Errorf("review changed at %s; reopen before editing", path)
}

// Comment and reply IDs let independent additions, edits, and deletions merge
// without treating a stale list as a replacement for newer threads.
func mergeEntries(base, local, fresh any, path string) (any, error) {
	indexed := make([]map[string]any, 3)
	var order []string
	seen := map[string]bool{}
	for i, value := range []any{base, local, fresh} {
		indexed[i] = map[string]any{}
		entries, _ := value.([]any)
		for _, entry := range entries {
			object, ok := entry.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("invalid review entry at %s", path)
			}
			id, ok := object["id"].(string)
			if !ok || id == "" || indexed[i][id] != nil {
				return nil, fmt.Errorf("invalid review entry ID at %s", path)
			}
			indexed[i][id] = entry
		}
	}
	for _, value := range []any{fresh, local, base} {
		entries, _ := value.([]any)
		for _, entry := range entries {
			id := entry.(map[string]any)["id"].(string)
			if !seen[id] {
				order = append(order, id)
				seen[id] = true
			}
		}
	}
	merged := []any{}
	for _, id := range order {
		entry, err := mergeValue(indexed[0][id], indexed[1][id], indexed[2][id], path+"["+id+"]")
		if err != nil {
			return nil, err
		}
		if entry != nil {
			merged = append(merged, entry)
		}
	}
	return merged, nil
}
