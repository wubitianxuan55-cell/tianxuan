package cli

import "charm.land/lipgloss/v2"

var (
	// Input box: only top + bottom borders, no sides.
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, true, false).
			BorderForeground(lipgloss.Color("173")).
			PaddingLeft(1)

	// Approval banner: same frame as the input box, recoloured yellow.
	approvalBannerStyle = lipgloss.NewStyle().
				Border(lipgloss.NormalBorder(), true, false, true, false).
				BorderForeground(lipgloss.Color("220")).
				Foreground(lipgloss.Color("220")).
				Bold(true).
				PaddingLeft(1)

	// Task panel: a top-bordered block pinned above the input.
	todoPanelStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(lipgloss.Color("240")).
			PaddingLeft(1)

	statusStyle = lipgloss.NewStyle().Faint(true)
)
