package ui

import (
	"fmt"
	"strings"
)

// ─── procedencia de parches ─────────────────────────────────────────
//
// En MOBA TUI conviven DOS identidades de parche distintas que nunca
// deben confundirse:
//
//  1. El PARCHE DE DATOS (estadísticas de partidas). En LoL lo publica
//     Lolalytics dentro del payload Qwik de cada campeón
//     (ChampionBuilds.Patch → CleanBuild.Patch → Plan.Patch). Describe
//     de qué parche son las partidas que se están contando.
//
//  2. El PARCHE DE CATÁLOGO (nombres, iconos y metadatos). En LoL es la
//     primera entrada de api/versions.json de Data Dragon
//     (Session.Version). También es la identidad con la que se cachea el
//     arte de ítems y runas.
//
// Que difieran es lo NORMAL: Data Dragon se publica con el parche nuevo
// antes de que Lolalytics acumule muestra suficiente, y las estadísticas
// se quedan un tiempo en el parche anterior. Antes, la cabecera mostraba
// b.Patch ("Patch 16.22") y el pie s.Version ("16.23.1") sin decir qué
// era cada uno: parecía una contradicción.
//
// Decisión de diseño: NO se sustituye un parche por el otro. Se
// etiqueta cada uno con su procedencia. Tampoco se deduce frescura del
// número de parche: eso ya lo hacen DataKind/DataAge con la caché.

// patchProvenance reúne las dos identidades y su relación.
type patchProvenance struct {
	// StatsPatch es el parche de las estadísticas de partidas.
	StatsPatch string
	// CatalogPatch es el parche del catálogo gráfico/de nombres.
	CatalogPatch string
}

// provenance construye la procedencia a partir de la sesión y la build
// en pantalla. Cualquiera de las dos puede faltar: nunca se inventa.
func (d detailView) provenance() patchProvenance {
	p := patchProvenance{CatalogPatch: d.session.Version}
	if b := d.CurrentBuild(); b != nil {
		p.StatsPatch = b.Patch
	}
	return p
}

// normalizePatch reduce un parche a su serie de parche (major.minor).
//
// "16.22", "16.22.1" y "16.22.10" pertenecen todos a la serie 16.22:
// Data Dragon publica un sufijo de revisión (16.23.1) y Lolalytics
// publica la serie (16.23). Comparar las cadenas literales diría que
// difieren aunque sean el mismo parche de juego.
//
// Lo que NO hace: ocultar diferencias materiales. Solo se considera
// igual la serie; 16.22 contra 16.23 sigue siendo distinto. Un valor
// no numérico se devuelve tal cual, para no convertirlo en "" y perder
// la única información disponible.
func normalizePatch(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	parts := strings.Split(v, ".")
	if len(parts) < 2 {
		return v
	}
	// Solo se recorta si las dos primeras partes son numéricas.
	for _, p := range parts[:2] {
		if p == "" || !isAllDigits(p) {
			return v
		}
	}
	return parts[0] + "." + parts[1]
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return len(s) > 0
}

// patchRelation clasifica la relación entre los dos parches.
type patchRelation int

const (
	// patchRelationUnknown: falta alguna de las dos identidades.
	patchRelationUnknown patchRelation = iota
	// patchRelationSame: misma serie de parche.
	patchRelationSame
	// patchRelationSeriesDiff: series distintas (16.22 vs 16.23). Es la
	// diferencia NORMAL entre fuente de estadísticas y catálogo; NO es
	// prueba de incompatibilidad.
	patchRelationSeriesDiff
)

func (p patchProvenance) relation() patchRelation {
	a, b := normalizePatch(p.StatsPatch), normalizePatch(p.CatalogPatch)
	if a == "" || b == "" {
		return patchRelationUnknown
	}
	if a == b {
		return patchRelationSame
	}
	return patchRelationSeriesDiff
}

// compactPatch: "16.22" → "16.22"; "16.23.1" → "16.23.1". Sin cambios
// más allá del recorte: la versión larga es informativa en el pie.
func compactPatch(v string) string {
	if v == "" {
		return "—"
	}
	return v
}

// headerPatchLabel es la etiqueta de parche de la cabecera del detalle.
// Ancho disponible: ~40 columnas de media en terminal estrecha.
//
// Prioridad cuando solo cabe una: la del conjunto estadístico que el
// jugador está viendo (las cifras de la cabecera salen de ahí).
func headerPatchLabel(p patchProvenance, wide bool) string {
	switch p.relation() {
	case patchRelationUnknown:
		if p.StatsPatch != "" {
			return "Datos " + compactPatch(p.StatsPatch)
		}
		if p.CatalogPatch != "" {
			return "Catálogo " + compactPatch(p.CatalogPatch)
		}
		return "parche desconocido"
	case patchRelationSame:
		if wide {
			return "Datos " + compactPatch(p.StatsPatch)
		}
		return "Datos " + compactPatch(p.StatsPatch)
	case patchRelationSeriesDiff:
		if !wide {
			return "Datos " + compactPatch(p.StatsPatch)
		}
		return "Datos " + compactPatch(p.StatsPatch) +
			" · Catálogo " + compactPatch(p.CatalogPatch) + " · parches distintos"
	default:
		return ""
	}
}

// footerPatchLabel es la etiqueta del pie. Aquí el parche protagonista
// es el del CATÁLOGO, porque el resto de la línea (campeones, builds)
// describe lo que la aplicación tiene cargado.
func footerPatchLabel(p patchProvenance, wide bool) string {
	if p.CatalogPatch == "" {
		if wide {
			return "Catálogo sin versión"
		}
		return "catálogo s/d"
	}
	if wide {
		return "Catálogo " + compactPatch(p.CatalogPatch)
	}
	// En estrecho, "Cat." mantiene la identidad legible sin comerse el
	// recuento de campeones.
	return "Cat. " + compactPatch(p.CatalogPatch)
}

// staleNote distingue "diferencia normal de versión" de "datos
// almacenados y posiblemente antiguos".
//
// La frescura NO se deduce del número de parche (un parche alto no
// implica datos recientes): se deduce de la caché, que es el único dato
// con fecha real. Esto solo añade la lectura de que las estadísticas
// vienen de un parche anterior al catálogo, que es descriptivo.
func staleNote(p patchProvenance) string {
	if p.relation() != patchRelationSeriesDiff {
		return ""
	}
	return "las estadísticas son de un parche anterior al catálogo"
}

// provenanceLine resume la procedencia para el popup de detalle, donde
// sí hay espacio para explicar la diferencia entre las dos identidades.
//
// Va en DOS líneas a propósito: "Briar · MIDDLE · datos 16.21 ·
// catálogo 16.23.1 · fuente lolalytics.com" no cabe en las 44 celdas
// útiles del popup y se partiría por la mitad de forma ilegible.
func provenanceLines(p patchProvenance, role, source, champ string) []string {
	primera := fmt.Sprintf("  build: %s · %s · fuente %s", champ, role, source)
	if source == "" {
		primera = fmt.Sprintf("  build: %s · %s", champ, role)
	}
	segunda := fmt.Sprintf("  parches: datos %s · catálogo %s",
		compactPatch(p.StatsPatch), compactPatch(p.CatalogPatch))
	return []string{primera, segunda}
}
