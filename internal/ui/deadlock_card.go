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

// dlRelation es la línea de contexto de la tarjeta: de qué mejora
// viene, o la etiqueta corta de la build híbrida.
func (d detailView) dlRelation(c cardItem) string {
	relation := ""
	if c.kind == kindUpgrade {
		relation = "↑ Mejora de " + c.baseName
	}
	// Build híbrida: la segunda línea de relación lleva la etiqueta
	// corta (confianza o función situacional), no una explicación.
	if c.hybrid {
		switch c.hybridSection {
		case SectionSituationals:
			relation = "[" + c.hybridReason + "]"
		case SectionExtensions:
			relation = "ampliación · " + ConfidenceWord(c.hybridConf)
		default:
			if c.kind == kindUpgrade {
				relation = "↑ " + c.baseName + " · " + ConfidenceWord(c.hybridConf)
			} else {
				relation = ConfidenceWord(c.hybridConf) + " · " + fmt.Sprintf("%.0f%%", c.hybridShare*100)
			}
		}
	}
	return relation
}

// dlItemCard reserves fixed slots so names and missing art cannot move the cost.
//
// Dos presentaciones:
//   - Normal: icono + nombre en dos líneas + relación en dos + coste (7 líneas).
//   - Compacta (viewport bajo): nombre, relación y coste en una línea cada
//     uno (4 líneas). Conserva icono, número de paso, mejora, coste, minuto
//     y confianza; solo se recorta el relleno.
func (d detailView) dlItemCard(idx int, c cardItem) string {
	inner := d.cardW() - 2
	name := d.session.ItemName(c.item.ID)
	heading := name
	if c.step > 0 {
		heading = fmt.Sprintf("%d %s", c.step, name)
	}
	marker := catColorStyle(d.session.ItemMeta[c.item.ID].Category).Render("● ")
	relation := d.dlRelation(c)
	timing := d.dlTimingCost(c)

	border := lipgloss.Color("#334155")
	if idx == d.itemIdx {
		border = colGold
	} else if d.relatedSel()[c.item.ID] {
		border = colGoldDim
	}

	var body []string
	if d.compactMode() {
		// Compacta: una línea por dato, sin relleno. El icono se omite
		// (ya lo hace el modo compacto) para no gastar altura.
		body = append(body,
			marker+stKeyName.Render(truncate(heading, inner-2)),
			stValue.Render(truncate(relation, inner)),
			stValue.Render(truncate(timing, inner)),
		)
	} else {
		icon := d.session.ItemArt[c.item.ID]
		if icon == "" {
			icon = d.fallbackIcon(name)
		}
		body = append(body, lipgloss.NewStyle().Width(inner).Height(6).
			MaxHeight(6).MaxWidth(inner).Align(lipgloss.Center).Render(icon))
		names := wrapName(heading, inner-2)
		for len(names) < 2 {
			names = append(names, "")
		}
		body = append(body, marker+stKeyName.Render(names[0]), "  "+stKeyName.Render(names[1]))
		relations := wrapName(relation, inner)
		for len(relations) < 2 {
			relations = append(relations, "")
		}
		body = append(body, stValue.Render(relations[0]), stValue.Render(relations[1]),
			stValue.Render(truncate(timing, inner)))
	}
	return lipgloss.NewStyle().Width(d.cardW()).Padding(0, 1).
		Border(lipgloss.NormalBorder()).BorderForeground(border).
		Render(strings.Join(body, "\n"))
}

func (d detailView) dlSelectionLine(c cardItem) string {
	width := max(24, d.width-10)
	heading := "▸ " + d.session.ItemName(c.item.ID)
	if c.step > 0 {
		heading = fmt.Sprintf("▸ Paso %d · %s", c.step, d.session.ItemName(c.item.ID))
	}
	context := d.dlTimingCost(c)
	if c.kind == kindUpgrade {
		context += " · Mejora " + c.baseName
	}
	return stKeyName.Render(truncate(heading, width)) + "\n" + stValue.Render(truncate(context, width))
}
