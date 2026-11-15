# Integración del generador híbrido en la TUI de Deadlock

Fecha: 2026-11-07. Rama `master`. Sin commits.
Datos: caché local (`data/cache/`), cero solicitudes HTTP nuevas.

---

## 1. Archivos modificados

### Nuevos
| Archivo | Qué es |
|---|---|
| `internal/ui/hybrid_integration_test.go` | 14 pruebas de integración TUI↔híbrido |
| `internal/deadlock/integration_heroes_test.go` | Los 4 héroes del brief, invariantes completas |
| `internal/deadlock/open_perf_test.go` | Tiempo de apertura completo de pantalla |
| `internal/ui/render_violet_test.go` | Render en texto de la pantalla real de Violet |

### Modificados
| Archivo | Cambio |
|---|---|
| `internal/ui/route.go` | `HybridStep`, `HybridBuildView`, `BuildLoader`, `hybridLoadedMsg` |
| `internal/ui/app.go` | `Session.Hybrids`/`HybridLoader`, `hybridCmdFor`, handler del mensaje |
| `internal/ui/detail.go` | `hybridCards`, `hybridSections`, `hybridHeader`, `ConfidenceWord`, popup con procedencia |
| `internal/ui/deadlock_card.go` | Etiqueta corta de confianza / función situacional |
| `internal/deadlock/hybrid.go` | `ProgressionCatalog`, anti-redundancia, ampliaciones solo LATE |
| `internal/deadlock/api.go` | Campos `progCat`/`progCatMu` |
| `internal/deadlock/hybrid_test.go` | 4 pruebas de regresión nuevas |
| `main.go` | `LoadHybrid` (adaptador `BuildHybrid` → UI) |

---

## 2. Cómo se conecta `BuildHybrid` con la TUI

Cadena completa, sin lógica estadística en la UI:

```
main.go: deadlockRuns.LoadHybrid(champKey)
  → api.FetchRuns            (muestra, memoizada)
  → api.ProgressionCatalog   (catálogo validado, memoizado)
  → deadlock.BuildHybrid     (motor de dominio)
  → ui.HybridBuildView       (datos planos)
       ↓
ui: hybridLoadedMsg → Session.Hybrids[key]
       ↓
detail.hybridCards()  → tarjetas
detail.hybridSections() → EARLY / MID / LATE / AMPLIACIONES / SITUACIONALES
```

Puntos de diseño:

- **La UI no calcula nada.** `HybridStep` es un struct plano con los valores
  ya resueltos. `internal/ui` no importa `internal/deadlock` ni
  `internal/progression`.
- **Fallback intacto.** Sin build híbrida (cargando, error, muestra corta) las
  tarjetas salen de la ruta observada anterior, sin cambios.
- **Carga en paralelo.** Al abrir el héroe se lanzan ruta e híbrida con
  `tea.Batch`; la primera que llega pinta.
- **Reutilización.** `Session.Hybrids` guarda el resultado por héroe: volver a
  un héroe no recalcula.
- **LoL no se toca.** El camino híbrido está detrás de
  `Game == GameDeadlock`; hay una prueba que lo verifica.

---

## 3. Verificaciones previas: problemas encontrados y corregidos

### 3.1 Superior Cooldown en ampliaciones (reportado)

**Causa:** Superior Cooldown es el componente directo de Transcendent Cooldown
(`upgrade_cooldown_reduction` → `upgrade_transcendent_cooldown`). Con la mejora
en la ruta, el componente no puede ser una compra adicional.

**Hallazgo adicional, más amplio:** el mismo bug afectaba a **Mystic Burst**
(componente de Tankbuster, en inventario) y **Compress Cooldown** (componente
de Superior Cooldown, en ruta).

**Corrección:** se excluye de ampliaciones y situacionales cualquier objeto que
sea componente transitivo de algo que la build ya compra.

**Prueba:** `TestHybridNoRedundantExtensions`.

### 3.2 Ampliaciones que no eran de partida larga

**Causa:** el filtro de ampliaciones solo miraba confianza, no fase. Extra Regen
(min 3) y Healing Booster (min 13) aparecían como "ampliaciones".

**Corrección:** una ampliación debe ser de fase LATE. Un objeto de EARLY que no
entró en la ruta es una compra con evidencia insuficiente, no una ampliación.

**Prueba:** `TestHybridExtensionsAreLateGame`.

Resultado en Violet: las 4 ampliaciones son ahora Spiritual Overflow (29 min),
Warp Stone (20 min), Spirit Burn y Torment Pulse.

### 3.3 Inventario válido tras cada compra

Verificado paso a paso con `TestHybridInventoryValidAfterEachStep`: sin
duplicados, sin exceder 12 espacios, el objeto comprado presente, los
componentes consumidos ausentes, ninguna compra repetida.

### 3.4 Progresión de fases realista

`TestHybridPhaseProgressionRealistic` e `TestIntegrationFourHeroes`: la fase no
retrocede a lo largo de la secuencia en ninguno de los cuatro héroes.

### 3.5 Etiquetas de situacionales

Las etiquetas salen de propiedades verificables del nombre del objeto
(Anticuración, Defensa contra balas, Defensa espiritual, Resistencia al control,
Supervivencia, Daño adicional). Ninguna afirma contrarrestar un héroe concreto.
`TestHybridSituationalLabelsShort` lo verifica.

---

## 4. Resultado final de Violet

Muestra: 250 partidas. Umbral 30 % conservado.

```
Build híbrida · 9 objetos finales · confianza moderada

◆ EARLY
  1 Grit          alta · 60%   ~4 min · 800 almas
  2 Extra Spirit  alta · 91%   ~5 min · 800 almas
  3 Mystic Regen. alta · 52%   ~5 min · 800 almas
  4 Extra Charge  alta · 94%   ~5 min · 800 almas

◆ MID
  5 Improved Spirit  ↑ Extra Spirit · alta      ~10 min · +800 almas
  6 Mystic Shot      moderada · 39%             ~11 min · 1600 almas
  7 Spirit Shielding ↑ Grit · alta              ~11 min · +800 almas
  8 Radiant Regen.   ↑ Mystic Regeneration·alta ~13 min · +2400 almas
  9 Healbane         moderada · 34%             ~13 min · 1600 almas
 10 Rapid Recharge   ↑ Extra Charge · alta      ~15 min · +2400 almas
 11 Spirit Lifesteal moderada · 34%             ~18 min · 1600 almas

◆ LATE
 12 Tankbuster          ↑ Mystic Burst · alta       ~23 min · +2400 almas
 13 Superior Cooldown   ↑ Compress Cooldown · alta  ~25 min · +1600 almas
 14 Boundless Spirit    ↑ Improved Spirit · alta    ~28 min · +4800 almas
 15 Transcendent Cooldown ↑ Superior Cooldown·mod.  ~29 min · +3200 almas

◆ AMPLIACIONES
  Spiritual Overflow · Warp Stone · Spirit Burn · Torment Pulse

◆ SITUACIONALES
  [Supervivencia] [Daño adicional] [Defensa contra balas]
  [Defensa espiritual] [Resistencia al control]
```

- 15 entradas → **9 objetos finales**
- 0 rechazos, 39 200 almas
- Numeración continua 1..15
- Transcendent Cooldown **dentro de la ruta** (antes solo en «Fuera de la ruta»)
- Boundless Spirit completa su cadena de tres

---

## 5. Comparación de la interfaz anterior y la nueva

| Aspecto | Antes | Ahora |
|---|---|---|
| Origen de las tarjetas | `dlBuildCards` (corte por fase, top-8) | `BuildHybrid` (por objeto final) |
| Objetos finales | 8 | 9 |
| Numeración | Hueco 7→9 | Continua 1..15 |
| Transcendent Cooldown | «Fuera de la ruta» | Paso 15 de la ruta |
| Cadenas | Cortadas en la base | Completas (Extra Spirit→Improved→Boundless) |
| Secciones | EARLY/MID/LATE + «Fuera de la ruta» | EARLY/MID/LATE + AMPLIACIONES + SITUACIONALES |
| Confianza | No visible | Palabra (alta/moderada/baja), nunca probabilidad |
| Procedencia | No visible | En el popup (Enter) |
| Situacionales | Lista plana sin función | Etiqueta breve por función |
| Cabecera | Explicaciones largas | Una línea: objetos finales + confianza |

---

## 6. Pruebas ejecutadas

`go test ./...` verde en los 14 paquetes con pruebas.
`go vet ./...` limpio. `gofmt -l` limpio. `git diff --check` limpio.

### Integración TUI (14 pruebas nuevas)
- `TestHybridSectionsVisible` — las cinco secciones en orden
- `TestHybridStepNumbersContinuous` — numeración 1..n, extras sin número
- `TestHybridCardsComeFromBuild` — las tarjetas vienen del híbrido
- `TestHybridFallsBackWithoutBuild` — fallback a la ruta observada
- `TestHybridConfidenceNotShownAsProbability` — sin % ni "probabilidad"
- `TestHybridSituationalLabelsShort` — etiquetas breves, sin héroes
- `TestHybridPopupShowsProvenance` — procedencia en el detalle
- `TestHybridSelectionAndScrollPreserved` — selección y scroll intactos
- `TestHybridNarrowTerminal` — 80×24, 120×40, 60×20, 40×16
- `TestHybridLoadingState` — "Calculando build…"
- `TestHybridErrorKeepsRoute` — error no rompe la vista
- `TestHybridDoesNotAffectLoL` — LoL sin marca híbrida
- `TestConfidenceWord` — umbrales documentados

### Motor (4 pruebas nuevas)
- `TestHybridNoRedundantExtensions`
- `TestHybridInventoryValidAfterEachStep`
- `TestHybridPhaseProgressionRealistic`
- `TestHybridExtensionsAreLateGame`

### Integración de héroes
- `TestIntegrationFourHeroes` — Violet, Solomon, Pocket, Celeste con 7 invariantes cada uno

### LoL
`lolalytics`, `ugg`, `ddragon`, `clean`, `model` y las pruebas de UI de LoL
pasan sin cambios.

---

## 7. Rendimiento de apertura de héroe

Medido con `TestOpenScreenTiming` (caché local, sin red):

| Escenario | Tiempo | Desglose |
|---|---|---|
| Apertura fría (Violet) | **62.9 ms** | catálogo 32.4 + runs 29.8 + build 0.7 |
| Reapertura (Violet) | **1.6 ms** | catálogo 0.0004 + runs 0.7 + build 0.9 |
| Otro héroe (Solomon) | **3.2 ms** | catálogo 0.0001 + runs 2.5 + build 0.7 |

**39× más rápido al reabrir.** La reapertura reutiliza catálogo de progresión,
catálogo de tienda, runs decodificadas y el resultado de la build guardado en
`Session.Hybrids`.

---

## 8. Problemas pendientes y limitaciones

1. **No hay validación visual real.** Todo lo anterior son pruebas
   automatizadas y un render en texto. **No se abrió la TUI en un terminal
   real.** El brief es explícito: no declarar validación visual realizada.
2. **Desborde de alto por debajo de 20 filas.** A 40×16 la vista ocupa 21
   líneas. Es **preexistente**: sin build híbrida da el mismo 21. No lo
   introduje y no lo corregí porque está fuera del alcance de esta entrega.
   El ancho (el requisito real de "terminal angosta") sí se verifica a todos
   los tamaños.
3. **El modo de comparación es implícito.** La ruta observada sigue
   calculándose y se usa como fallback, pero no hay un toggle explícito para
   ver "ruta observada vs build híbrida" lado a lado. Sería el siguiente paso
   natural.
4. **La confianza de Violet es 0.61 (moderada).** Es honesto: 250 partidas no
   sostienen más.
5. **El clasificador de situacionales es por nombre.** Un objeto nuevo sin
   palabra clave queda sin etiqueta y no se propone.
6. **La procedencia `community` no se usa.** No hay fuentes comunitarias
   verificables en la caché y no se inventaron.

---

## 9. Propuesta de commits separados (sin ejecutar)

```
1. feat(ui): tipos de la build híbrida y loader
   internal/ui/route.go

2. feat(deadlock): catálogo de progresión memoizado y anti-redundancia
   internal/deadlock/hybrid.go
   internal/deadlock/api.go
   internal/deadlock/hybrid_test.go

3. feat(ui): tarjetas y secciones desde la build híbrida
   internal/ui/detail.go
   internal/ui/deadlock_card.go

4. feat(ui): carga de la build híbrida al abrir el héroe
   internal/ui/app.go
   main.go

5. test(ui): integración TUI↔híbrido
   internal/ui/hybrid_integration_test.go

6. test(deadlock): cuatro héroes, apertura y render
   internal/deadlock/integration_heroes_test.go
   internal/deadlock/open_perf_test.go
   internal/ui/render_violet_test.go
```

Los cambios preexistentes del working tree (`README.md`, `app.go` base,
`route.go` base, `main.go` base, tests de viewport) siguen sin commitear y
deben revisarse por separado.

---

## Criterio de cierre

La build híbrida **aparece realmente en la TUI**: se ve, se navega, se
selecciona, se abre su detalle y sobrevive al scroll. Conserva una progresión
válida (inventario comprobado paso a paso, cadenas completas, sin
redundancias) y elimina la necesidad de consultar «Fuera de la ruta» para
completar el equipamiento: Transcendent Cooldown, Boundless Spirit y Healbane
están ahora en la ruta principal.

Lo que **no** se afirma: que la interfaz se haya visto en un terminal real.
