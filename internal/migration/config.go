package migration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func convertConfig(m move) (err error) {
	info, err := os.Lstat(m.from)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("config path is not a regular file: %s", m.from)
	}
	data, err := os.ReadFile(m.from)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var doc map[string]any
	if err := decoder.Decode(&doc); err != nil {
		return err
	}
	if doc == nil {
		return fmt.Errorf("config must be a JSON object")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("config contains multiple JSON values")
		}
		return err
	}
	value, err := configValue(doc)
	if err != nil {
		return err
	}
	encoded, err := toml.Marshal(value)
	if err != nil {
		return err
	}
	var check map[string]any
	if err := toml.Unmarshal(encoded, &check); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(m.to), ".config-migrating-")
	if err != nil {
		return err
	}
	defer func() {
		if removeErr := os.Remove(file.Name()); !os.IsNotExist(removeErr) {
			err = errors.Join(err, removeErr)
		}
	}()
	err = file.Chmod(info.Mode().Perm())
	if err == nil {
		_, err = file.Write(encoded)
	}
	if err == nil {
		err = file.Sync()
	}
	err = errors.Join(err, file.Close())
	if err != nil {
		return err
	}
	if err := renameAbsent(file.Name(), m.to); err != nil {
		return err
	}
	return os.Remove(m.from)
}

func configValue(value any) (any, error) {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			converted, err := configValue(item)
			if err != nil {
				return nil, fmt.Errorf("key %q: %w", key, err)
			}
			value[key] = converted
		}
	case []any:
		for i, item := range value {
			converted, err := configValue(item)
			if err != nil {
				return nil, fmt.Errorf("element %d: %w", i, err)
			}
			value[i] = converted
		}
	case json.Number:
		if !strings.ContainsAny(string(value), ".eE") {
			return strconv.ParseInt(string(value), 10, 64)
		}
		return strconv.ParseFloat(string(value), 64)
	case nil:
		return nil, fmt.Errorf("TOML cannot represent null; remove or replace it before migration")
	}
	return value, nil
}
