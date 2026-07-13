package config

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// RenderTOML renders the config as annotated TOML in the `tianxuan setup` house style:
// comments preserved, system_prompt as a multi-line string, helpful hints. The
// output round-trips back through Load (see render_test.go).
func RenderTOML(c *Config) string {
	var b strings.Builder

	b.WriteString("# Tianxuan configuration.\n")
	b.WriteString("# Resolution order: flag > ./tianxuan.toml > ~/.config/tianxuan/config.toml > built-in defaults.\n")
	b.WriteString("# Secrets come from the environment via api_key_env; never put keys here.\n\n")

	fmt.Fprintf(&b, "default_model = %q\n", c.DefaultModel)
	if c.Language != "" {
		fmt.Fprintf(&b, "language      = %q   # ui/model language; empty = auto-detect from $LANG / $TIANXUAN_LANG\n", c.Language)
	} else {
		b.WriteString("# language      = \"zh\"   # ui/model language; empty = auto-detect from $LANG / $TIANXUAN_LANG\n")
	}
	b.WriteString("\n")

	b.WriteString("[agent]\n")
	b.WriteString("system_prompt = \"\"\"\n")
	b.WriteString(c.Agent.SystemPrompt)
	b.WriteString("\"\"\"\n")
	if c.Agent.SystemPromptFile != "" {
		fmt.Fprintf(&b, "system_prompt_file = %q\n", c.Agent.SystemPromptFile)
	} else {
		b.WriteString("# system_prompt_file = \"prompts/system.md\"   # overrides system_prompt when set\n")
	}
	fmt.Fprintf(&b, "max_steps   = %d\n", c.Agent.MaxSteps)
	fmt.Fprintf(&b, "temperature = %s\n", formatFloat(c.Agent.Temperature))
	if c.Agent.PlannerTemperature > 0 {
		fmt.Fprintf(&b, "planner_temperature = %s   # override for Hermes planner (0 = use temperature)\n", formatFloat(c.Agent.PlannerTemperature))
	}
	if c.Agent.SubagentTemperature > 0 {
		fmt.Fprintf(&b, "subagent_temperature = %s   # override for task sub-agents (0 = use temperature)\n", formatFloat(c.Agent.SubagentTemperature))
	}
	if c.Agent.Effort != "" {
		fmt.Fprintf(&b, "effort = %q   # executor reasoning: high | max (empty = off)\n", c.Agent.Effort)
	}
	if c.Agent.PlannerEffort != "" {
		fmt.Fprintf(&b, "planner_effort = %q   # override for Hermes (empty = inherit effort)\n", c.Agent.PlannerEffort)
	}
	if c.Agent.SubagentEffort != "" {
		fmt.Fprintf(&b, "subagent_effort = %q   # override for sub-agents (empty = inherit effort)\n", c.Agent.SubagentEffort)
	}
	if c.Agent.PlannerModel != "" {
		fmt.Fprintf(&b, "planner_model = %q   # low-frequency planner (two-model collaboration)\n", c.Agent.PlannerModel)
	} else {
		b.WriteString("# planner_model = \"mimo\"   # optional: enable two-model collaboration\n")
	}
	if c.Agent.PlannerMaxSteps > 0 {
		fmt.Fprintf(&b, "planner_max_steps = %d   # max planner tool-call rounds per turn\n", c.Agent.PlannerMaxSteps)
	} else {
		b.WriteString("# planner_max_steps = 12   # cap planner turns (0 = unlimited)\n")
	}
	if c.Agent.SubagentModel != "" {
		fmt.Fprintf(&b, "subagent_model = %q   # default model for runAs=subagent skills\n", c.Agent.SubagentModel)
	} else {
		b.WriteString("# subagent_model = \"deepseek-pro\"   # optional default for runAs=subagent skills\n")
	}
	if len(c.Agent.SubagentModels) > 0 {
		fmt.Fprintf(&b, "subagent_models = %s   # per-skill overrides\n", renderStringMap(c.Agent.SubagentModels))
	} else {
		b.WriteString("# subagent_models = { review = \"deepseek-pro\", security_review = \"deepseek-pro\" }   # per-skill overrides\n")
	}
	if c.Agent.OutputStyle != "" {
		fmt.Fprintf(&b, "output_style = %q   # persona/tone folded into the prompt\n", c.Agent.OutputStyle)
	} else {
		b.WriteString("# output_style = \"explanatory\"   # explanatory | learning | concise | custom; empty = default\n")
	}
	if c.Agent.MaxSubagentDepth > 0 {
		fmt.Fprintf(&b, "max_subagent_depth = %d   # cap recursion for runAs=subagent (0 = unlimited)\n", c.Agent.MaxSubagentDepth)
	} else {
		b.WriteString("# max_subagent_depth = 2   # cap recursion for runAs=subagent (0 = unlimited)\n")
	}
	if c.Agent.ColdResumePrune != nil && *c.Agent.ColdResumePrune {
		b.WriteString("cold_resume_prune = true   # trim expired tool results on cold resume\n")
	} else {
		b.WriteString("# cold_resume_prune = false   # trim expired tool results on cold resume\n")
	}
	if c.Agent.ReasoningLanguage != "" {
		fmt.Fprintf(&b, "reasoning_language = %q   # zh | en | auto\n", c.Agent.ReasoningLanguage)
	} else {
		b.WriteString("# reasoning_language = \"auto\"   # zh | en | auto; empty = auto-detect\n")
	}
	b.WriteString("\n")

	for _, p := range c.Providers {
		b.WriteString("[[providers]]\n")
		fmt.Fprintf(&b, "name        = %q\n", p.Name)
		fmt.Fprintf(&b, "kind        = %q\n", p.Kind)
		fmt.Fprintf(&b, "base_url    = %q\n", p.BaseURL)
		if len(p.Models) > 0 {
			fmt.Fprintf(&b, "models      = %s\n", renderStringArray(p.Models))
			if p.Default != "" {
				fmt.Fprintf(&b, "default     = %q\n", p.Default)
			}
		} else if p.Model != "" {
			fmt.Fprintf(&b, "model       = %q\n", p.Model)
		}
		fmt.Fprintf(&b, "api_key_env = %q\n", p.APIKeyEnv)
		if p.BalanceURL != "" {
			fmt.Fprintf(&b, "balance_url = %q   # optional; wallet-balance endpoint shown in the status bar\n", p.BalanceURL)
		}
		if p.ContextWindow > 0 {
			fmt.Fprintf(&b, "context_window = %d   # tokens; compaction triggers near this limit\n", p.ContextWindow)
		}
		if p.Price != nil {
			fmt.Fprintf(&b, "price       = { cache_hit = %v, input = %v, output = %v, currency = %q }   # per 1M tokens\n",
				p.Price.CacheHit, p.Price.Input, p.Price.Output, p.Price.Symbol())
		}
		if p.Thinking != "" {
			fmt.Fprintf(&b, "thinking    = %q\n", p.Thinking)
		}
		if p.Effort != "" {
			fmt.Fprintf(&b, "effort      = %q\n", p.Effort)
		}
		if p.ChatURL != "" {
			fmt.Fprintf(&b, "chat_url    = %q\n", p.ChatURL)
		}
		if p.ModelsURL != "" {
			fmt.Fprintf(&b, "models_url  = %q\n", p.ModelsURL)
		}
		if len(p.Headers) > 0 {
			fmt.Fprintf(&b, "headers     = %s\n", renderStringMap(p.Headers))
		}
		if p.ExtraBody != "" {
			fmt.Fprintf(&b, "extra_body  = %q\n", p.ExtraBody)
		}
		if p.AuthHeader {
			b.WriteString("auth_header = true\n")
		}
		if len(p.VisionModels) > 0 {
			fmt.Fprintf(&b, "vision_models = %s\n", renderStringArray(p.VisionModels))
		}
		if p.ReasoningProtocol != "" {
			fmt.Fprintf(&b, "reasoning_protocol = %q\n", p.ReasoningProtocol)
		}
		if len(p.SupportedEfforts) > 0 {
			fmt.Fprintf(&b, "supported_efforts = %s\n", renderStringArray(p.SupportedEfforts))
		}
		if p.DefaultEffort != "" {
			fmt.Fprintf(&b, "default_effort = %q\n", p.DefaultEffort)
		}
		b.WriteString("\n")
	}

	b.WriteString("[tools]\n")
	if len(c.Tools.Enabled) == 0 {
		b.WriteString("enabled = []   # empty = all built-in tools\n\n")
	} else {
		b.WriteString("enabled = [")
		for i, t := range c.Tools.Enabled {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%q", t)
		}
		b.WriteString("]\n\n")
	}

	b.WriteString("[permissions]\n")
	b.WriteString("# Per-call gating. mode = writer fallback when no rule matches: ask|allow|deny.\n")
	b.WriteString("# Readers always default to allow. Precedence: deny > ask > allow > fallback.\n")
	b.WriteString("# Rules are \"ToolName\" or \"ToolName(glob)\"; '*' matches any run, '?' one char.\n")
	mode := c.Permissions.Mode
	if mode == "" {
		mode = "ask"
	}
	fmt.Fprintf(&b, "mode  = %q\n", mode)
	b.WriteString(renderRuleList("deny", c.Permissions.Deny, `["bash(rm -rf*)", "bash(git push*)"]   # hard-blocked in every mode`))
	b.WriteString(renderRuleList("allow", c.Permissions.Allow, `["bash(go test*)", "bash(git status*)"]   # never prompted`))
	b.WriteString(renderRuleList("ask", c.Permissions.Ask, `["write_file"]   # force a prompt even if otherwise allowed`))
	b.WriteString("\n")

	b.WriteString("[sandbox]\n")
	b.WriteString("# Confine tool blast radius. File-writers (write_file/edit_file/multi_edit)\n")
	b.WriteString("# may only write under workspace_root (empty = current dir) + allow_write.\n")
	b.WriteString("# bash = \"enforce\" (default) jails each command in an OS sandbox (macOS now;\n")
	b.WriteString("# graceful fallback elsewhere); \"off\" disables it. network allows egress.\n")
	if c.Sandbox.WorkspaceRoot != "" {
		fmt.Fprintf(&b, "workspace_root = %q\n", c.Sandbox.WorkspaceRoot)
	} else {
		b.WriteString("# workspace_root = \"\"            # default: current working directory\n")
	}
	if len(c.Sandbox.AllowWrite) > 0 {
		fmt.Fprintf(&b, "allow_write = %s\n", renderStringArray(c.Sandbox.AllowWrite))
	} else {
		b.WriteString("# allow_write = [\"/tmp\"]          # extra dirs writers may also modify\n")
	}
	fmt.Fprintf(&b, "bash    = %q\n", c.BashMode())
	fmt.Fprintf(&b, "network = %v\n", c.Sandbox.Network)
	b.WriteString("\n")

	b.WriteString("[statusline]\n")
	b.WriteString("# A custom status line: a command whose first stdout line replaces the built-in\n")
	b.WriteString("# data row. It receives {\"model\",\"contextUsed\",\"contextWindow\"} as JSON on stdin.\n")
	if c.Statusline.Command != "" {
		fmt.Fprintf(&b, "command = %q\n", c.Statusline.Command)
	} else {
		b.WriteString("# command = \"my-statusline.sh\"\n")
	}
	b.WriteString("\n")

	b.WriteString("# External MCP servers. type: \"stdio\" (default, a subprocess) | \"http\" | \"sse\".\n")
	b.WriteString("# ${VAR} / ${VAR:-default} are expanded from the environment in command/args/env/url/headers.\n")
	if len(c.Plugins) == 0 {
		b.WriteString("# [[plugins]]\n")
		b.WriteString("# name    = \"example\"\n")
		b.WriteString("# command = \"tianxuan-plugin-example\"\n")
		b.WriteString("# [[plugins]]                                  # a remote server over Streamable HTTP\n")
		b.WriteString("# name    = \"stripe\"\n")
		b.WriteString("# type    = \"http\"\n")
		b.WriteString("# url     = \"https://mcp.stripe.com\"\n")
		b.WriteString("# headers = { Authorization = \"Bearer ${STRIPE_KEY}\" }\n")
	} else {
		for _, pl := range c.Plugins {
			b.WriteString("\n[[plugins]]\n")
			fmt.Fprintf(&b, "name    = %q\n", pl.Name)
			if pl.Type != "" {
				fmt.Fprintf(&b, "type    = %q\n", pl.Type)
			}
			if pl.Command != "" {
				fmt.Fprintf(&b, "command = %q\n", pl.Command)
			}
			if len(pl.Args) > 0 {
				fmt.Fprintf(&b, "args    = %s\n", renderStringArray(pl.Args))
			}
			if pl.URL != "" {
				fmt.Fprintf(&b, "url     = %q\n", pl.URL)
			}
			if len(pl.Headers) > 0 {
				fmt.Fprintf(&b, "headers = %s\n", renderStringMap(pl.Headers))
			}
			if len(pl.Env) > 0 {
				fmt.Fprintf(&b, "env     = %s\n", renderStringMap(pl.Env))
			}
			if pl.AutoStart != nil {
				fmt.Fprintf(&b, "auto_start = %v\n", *pl.AutoStart)
			}
		}
	}

	return b.String()
}

// renderStringArray renders a []string as a TOML inline array.
func renderStringArray(ss []string) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, s := range ss {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", s)
	}
	b.WriteByte(']')
	return b.String()
}

// renderStringMap renders a map[string]string as a TOML inline table with keys
// in sorted order so output is deterministic (round-trips cleanly).
func renderStringMap(m map[string]string) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("{ ")
	for i, k := range keys {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%s = %q", k, m[k])
	}
	b.WriteString(" }")
	return b.String()
}

// renderRuleList emits a permission rule list. A populated list renders as an
// active TOML array; an empty one renders as a commented example so `tianxuan setup`
// scaffolds discoverable guidance without imposing surprising rules.
func renderRuleList(key string, rules []string, example string) string {
	if len(rules) == 0 {
		return fmt.Sprintf("# %s = %s\n", key, example)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s = [", key)
	for i, r := range rules {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", r)
	}
	b.WriteString("]\n")
	return b.String()
}

// formatFloat ensures a float renders with a decimal point so TOML types it as a
// float, not an integer (e.g. 0 -> "0.0").
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}
