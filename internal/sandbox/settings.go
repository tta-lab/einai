package sandbox

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func readOrInitSettings(settingsPath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]interface{}{}, nil
		}
		return nil, fmt.Errorf("reading settings.json: %w", err)
	}
	var settings map[string]interface{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parsing settings.json: %w", err)
	}
	return settings, nil
}

func extractPermsDenyList(settings map[string]interface{}) (map[string]interface{}, []interface{}, error) {
	var perms map[string]interface{}
	if raw, ok := settings["permissions"]; ok {
		perms, ok = raw.(map[string]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("settings.json: permissions is not an object (got %T)", raw)
		}
	}
	if perms == nil {
		perms = map[string]interface{}{}
	}

	var denySlice []interface{}
	if raw, ok := perms["deny"]; ok {
		denySlice, ok = raw.([]interface{})
		if !ok {
			return nil, nil, fmt.Errorf("settings.json: permissions.deny is not an array (got %T)", raw)
		}
	}
	return perms, denySlice, nil
}

func writeSettingsJSON(path string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
