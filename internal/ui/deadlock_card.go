package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// dlTimingCost keeps the same purchase cue on the card and in the footer.
func (d detailView) dlTimingCost(c cardItem) string {
	minute := "min —"
	if seconds := c.dlMinuteOf(); seconds > 0 {
		minute = fmt.Sprintf("~%d min", seconds/60)
	}
	cost := "coste —"
	if info, ok := d.session.ItemMeta[c.item.ID]; ok && info.Cost > 0 {
		if c.kind == kindUpgrade {
			if base, ok := d.session.ItemMeta[c.baseID]; ok && base.Cost > 0 && info.Cost > base.Cost {
				cost = fmt.Sprintf("+%d almas", info.Cost-base.Cost)
			}
		} else {
			cost = fmt.Sprintf("%d almas", info.Cost)
		}
	}
	return minute + " · " + cost
}

// dlItemCard reserves fixed slots so names and missing art cannot move the cost.
func (d detailView) dlItemCard(idx int, c cardItem) string {
	inner := d.cardW() - 2
	name := d.session.ItemName(c.item.ID)
	var body []string
	if !d.compactMode() {
		icon := d.session.ItemArt[c.item.ID]
		if icon == "" {
			icon = d.fallbackIcon(name)
		}
		body = append(body, lipgloss.NewStyle().Width(inner).Height(6).
			MaxHeight(6).MaxWidth(inner).Align(lipgloss.Center).Render(icon))
	}
	heading := name
	if c.order > 0 {
		heading = fmt.Sprintf("%d %s", c.order, name)
	}
	names := wrapName(heading, inner-2)
	for len(names) < 2 {
		names = append(names, "")
	}
	marker := catColorStyle(d.session.ItemMeta[c.item.ID].Category).Render("● ")
	body = append(body, marker+stKeyName.Render(names[0]), "  "+stKeyName.Render(names[1]))
	relation := ""
	if c.kind == kindUpgrade {
		relation = "↑ Mejora de " + c.baseName
	}
	relations := wrapName(relation, inner)
	for len(relations) < 2 {
		relations = append(relations, "")
	}
	body = append(body, stValue.Render(relations[0]), stValue.Render(relations[1]),
		stValue.Render(truncate(d.dlTimingCost(c), inner)))
	border := lipgloss.Color("#334155")
	if idx == d.itemIdx {
		border = colGold
	} else if d.relatedSel()[c.item.ID] {
		border = colGoldDim
	}
	return lipgloss.NewStyle().Width(d.cardW()).Padding(0, 1).
		Border(lipgloss.NormalBorder()).BorderForeground(border).
		Render(strings.Join(body, "\n"))
}

func (d detailView) dlSelectionLine(c cardItem) string {
	width := max(24, d.width-10)
	heading := "▸ " + d.session.ItemName(c.item.ID)
	if c.order > 0 {
		heading = fmt.Sprintf("▸ Paso %d · %s", c.order, d.session.ItemName(c.item.ID))
	}
	context := d.dlTimingCost(c)
	if c.kind == kindUpgrade {
		context += " · Mejora " + c.baseName
	}
	return stKeyName.Render(truncate(heading, width)) + "\n" + stValue.Render(truncate(context, width))
}
