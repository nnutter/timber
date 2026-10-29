package display

import (
	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

var (
	// StatusStyle renders success messages in green.
	StatusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	// WarningStyle renders warnings in yellow.
	WarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
)

// NewOutputTable builds a bordered table with padded cells and a bold
// header row.
func NewOutputTable(headers ...string) *table.Table {
	return table.New().
		Headers(headers...).
		Border(lipgloss.NormalBorder()).
		BorderHeader(true).
		StyleFunc(func(row int, column int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).PaddingLeft(1).PaddingRight(1)
			}
			return lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
		})
}
