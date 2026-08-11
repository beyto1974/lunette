package tui

import "charm.land/lipgloss/v2"

type styles struct {
	titleBar     lipgloss.Style
	fileName     lipgloss.Style
	meta         lipgloss.Style
	warn         lipgloss.Style
	paneActive   lipgloss.Style
	paneIdle     lipgloss.Style
	paneTitle    lipgloss.Style
	itemSelected lipgloss.Style
	itemNormal   lipgloss.Style
	itemOrdinal  lipgloss.Style
	itemMatch    lipgloss.Style
	itemYear     lipgloss.Style
	status       lipgloss.Style
	prompt       lipgloss.Style
}

func newStyles() styles {
	border := lipgloss.RoundedBorder()
	return styles{
		titleBar:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39")),
		fileName:     lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		meta:         lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		warn:         lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		paneActive:   lipgloss.NewStyle().Border(border).BorderForeground(lipgloss.Color("39")),
		paneIdle:     lipgloss.NewStyle().Border(border).BorderForeground(lipgloss.Color("240")),
		paneTitle:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("108")),
		itemSelected: lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true),
		itemNormal:   lipgloss.NewStyle().Foreground(lipgloss.Color("252")),
		itemOrdinal:  lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		itemMatch:    lipgloss.NewStyle().Foreground(lipgloss.Color("232")).Background(lipgloss.Color("220")),
		itemYear:     lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		status:       lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		prompt:       lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Bold(true),
	}
}
