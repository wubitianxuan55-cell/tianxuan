package builtin

import "tianxuan/internal/config"

// searchCfg holds the runtime search configuration injected by boot.
// nil means all API-based engines are disabled; only public SearXNG works.
var searchCfg *config.SearchConfig

// SetSearchConfig injects the search configuration from boot assembly.
// Call once before any web_search tool execution.
func SetSearchConfig(cfg config.SearchConfig) {
	cp := cfg
	searchCfg = &cp
}
