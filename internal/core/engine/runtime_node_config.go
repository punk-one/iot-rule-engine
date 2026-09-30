package engine

func cloneNodeRuntimeConfig(config map[string]interface{}) map[string]interface{} {
	if config == nil {
		return nil
	}

	cloned := make(map[string]interface{}, len(config))
	for key, value := range config {
		cloned[key] = cloneRuntimeConfigValue(value)
	}
	return cloned
}

func cloneRuntimeConfigValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneNodeRuntimeConfig(typed)
	case []interface{}:
		cloned := make([]interface{}, len(typed))
		for index, item := range typed {
			cloned[index] = cloneRuntimeConfigValue(item)
		}
		return cloned
	default:
		return typed
	}
}
