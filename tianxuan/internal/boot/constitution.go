package boot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// loadConstitution reads .tianxuan/constitution.toml and returns a formatted
// constitution block for the system prompt, or "" when the file doesn't exist.
func loadConstitution() string {
	path := filepath.Join(".tianxuan", "constitution.toml")
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	var cfg struct {
		AuthorityChain []string `toml:"authority_chain"`
		Constitution  struct {
			ProtectedInvariants []string `toml:"protected_invariants"`
		} `toml:"constitution"`
		VerificationPolicy string `toml:"verification_policy"`
	}
	if err := toml.Unmarshal(b, &cfg); err != nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("## Constitution \u2014 Project Inviolable Rules\n")
	sb.WriteString("These rules take precedence over all other instructions.\n\n")

	if len(cfg.AuthorityChain) > 0 {
		sb.WriteString("### Authority Chain (highest to lowest)\n")
		for i, a := range cfg.AuthorityChain {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, a)
		}
		sb.WriteString("\n")
	}

	if len(cfg.Constitution.ProtectedInvariants) > 0 {
		sb.WriteString("### Protected Invariants (never violate)\n")
		for _, inv := range cfg.Constitution.ProtectedInvariants {
			sb.WriteString("- " + inv + "\n")
		}
		sb.WriteString("\n")
	}

	if cfg.VerificationPolicy != "" {
		sb.WriteString("### Verification Policy\n")
		sb.WriteString(cfg.VerificationPolicy + "\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}
