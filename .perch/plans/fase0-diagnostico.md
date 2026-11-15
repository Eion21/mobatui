# Fase 0 — Diagnóstico reproducible

Fecha: 2026-11-07. Rama `master`, working tree con cambios preexistentes
preservados (8 archivos modificados + 4 nuevos, sin commitear).

## Entorno

- Go 1.27.3 en `~/.local/go/bin` (fuera del PATH de la shell; se exporta explícitamente).
- `go build ./...` limpio. `go test ./...` verde en los 14 paquetes con pruebas.
- `git diff --check` limpio.
- Datos: `data/cache/` contiene respuestas reales de `api.deadlock-api.com`
  (heroes, items, item-stats, ability-order-stats, matches/metadata) para
  Violet (86), Solomon (85), Pocket (50), Celeste (81) y otros.
- Todo el diagnóstico se hizo **offline** (`cache.Offline = true`). Cero
  solicitudes HTTP nuevas.

## Arnés reproducible

- `internal/deadlock/diag_baseline_test.go` — `TestDiagBaseline`: ruta de
  Violet con el algoritmo actual, inventario final, candidatos fuera de ruta.
- `internal/ui/diag_cards_test.go` — `TestDiagVioletCards`: tarjetas que la
  TUI realmente muestra, con numeración y huecos.
  `TestDiagPhaseCut`: ranking por fase antes del corte, con el puesto de cada
  compra y si queda cortada.

Ambos leen solo de `data/cache` y `testdata/`, sin red.

## Línea base de Violet

Muestra: 250 partidas. Umbral `minShare = 0.30`. Ruta: **17 pasos**.

| # | objeto | share | min | cadena |
|---|--------|-------|-----|--------|
| 1 | Extra Spirit | 0.91 | 5 | — |
| 2 | Grit | 0.60 | 4 | — |
| 3 | Extra Charge | 0.94 | 5 | — |
| 4 | Mystic Regeneration | 0.52 | 5 | — |
| 5 | Improved Spirit | 0.87 | 10 | de Extra Spirit (94 %) |
| 6 | Spirit Shielding | 0.53 | 11 | de Grit (65 %) |
| 7 | Mystic Shot | 0.39 | 11 | — |
| 8 | Healbane | 0.34 | 13 | — |
| 9 | Radiant Regeneration | 0.51 | 13 | de Mystic Regeneration (91 %) |
| 10 | Mystic Burst | 0.39 | 15 | — |
| 11 | Rapid Recharge | 0.87 | 15 | de Extra Charge (99 %) |
| 12 | Spirit Lifesteal | 0.34 | 18 | — |
| 13 | Compress Cooldown | 0.36 | 22 | — |
| 14 | Tankbuster | 0.58 | 23 | de Mystic Burst (59 %) |
| 15 | Superior Cooldown | 0.57 | 25 | de Compress Cooldown (58 %) |
| 16 | Boundless Spirit | 0.56 | 28 | de Improved Spirit (95 %) |
| 17 | Transcendent Cooldown | 0.32 | 29 | de Superior Cooldown (90 %) |

Tarjetas visibles en la TUI: **15** (no 17). Inventario final simulado:
**8 objetos** — Boundless Spirit, Spirit Shielding, Mystic Shot, Radiant
Regeneration, Rapid Recharge, Spirit Lifesteal, Tankbuster, Superior Cooldown.

Esto **reproduce exactamente** el fixture visual reportado: 15 compras, 8
objetos finales, hueco 7→9. El diagnóstico es fiel al síntoma.

## Causas raíz

### 1. Hueco de numeración 7 → 9

`cardItem.order` es el índice de la ruta (`dlDecorate`, `detail.go:358`), no
un número de paso de la presentación. `dlBuildCards` corta a `perPhase = 8`
tarjetas por fase (`detail.go:425`) y luego `filterUpgrades` puede quitar más.
Al cortar se pierde el paso 8 (**Healbane**) pero los supervivientes conservan
su `order` original: 7 → 9.

Evidencia (`TestDiagPhaseCut`, fase MID): Healbane tiene `ruta#8` y queda en
el **puesto 12 de 18** por score, por debajo del corte de 8. Causa: su
`rel = 0.24` (juegos relativos al ítem más comprado) es bajo comparado con
Healing Booster (0.70) o Extra Stamina (0.30), que no pertenecen a la ruta.

### 2. Transcendent Cooldown fuera de la ruta principal

Mismo mecanismo: `ruta#17`, `rel = 0.30`, **puesto 10 de 17** en LATE. Cortado
por `perPhase = 8`. Es la "compra tardía con posible relevancia" que el usuario
ve solo en «Fuera de la ruta».

### 3. Pocos objetos finales

El corte por fase es por *tarjeta*, no por *objeto final*. Una cadena de tres
(Extra Spirit → Improved Spirit → Boundless Spirit) consume tres tarjetas para
producir un objeto. Con 15 tarjetas se obtienen 8 objetos. Además, la
puntuación `2·√rel + 6·sig` premia popularidad y significación pero **no
premia completar cadenas**: un componente intermedio compite en igualdad con
un objeto final.

### 4. Habilidades de Solomon invertidas

`buildSkillSequences` (`api.go:694`) asigna los slots **por frecuencia de uso**
(`sort.Slice` sobre `use[id]`), no por el orden canónico del héroe. Con los
datos reales de Solomon (878 filas, 4 habilidades):

| slot asignado | habilidad | firma canónica |
|---|---|---|
| 1 | Rook Battery | signature3 |
| 2 | Develop Queen | signature4 |
| 3 | Setup Bishop | signature1 |
| 4 | Kingside Knight | signature2 |

El resultado es exactamente la inversión reportada (1↔4, 2↔3). La causa no es
un array invertido: es que el slot se deriva de la popularidad, y en Solomon la
habilidad 3 es la más subida. El orden canónico sí existe en el dato:
`heroes.json` → `items.signature1..4` (`ability_chessmaster_throwbishop`,
`knightbuff`, `forkpiece`, `queen`), y `items.json` resuelve cada `class_name`
a su `id` de habilidad.

### 5. Esperas repetidas al consultar héroes

`LoadRuns` (`main.go:125`) se dispara al abrir el detalle. `session.Routes`
guarda solo el resultado de la ruta, y `app.go:897` no vuelve a pedirla si ya
existe la clave. Pero `FetchRuns` reconstruye la ruta desde las `Run`
agregadas en cada llamada, y `LoadBuilds` recorre los 65 héroes en cada carga
completa. El snapshot estático (`staticSnapshot`, cambio preexistente en
`api.go`) ya evita repetir heroes/items dentro de una carga, pero no entre
cargas.

## Qué se recalcula al reabrir un héroe

- `Routes[key]` presente → no se relanza la carga (correcto).
- `Routes[key] == nil` → marca "en curso" y lanza `LoadRuns`.
- `LoadBuilds` completo → recorre hero-stats, item-stats y ability-order-stats
  de todos los héroes; el snapshot solo cubre assets estáticos.

## Compras excluidas y por qué

Fuera de la ruta con share ≥ 10 % (candidatos reales, no inventados):

Healing Booster 0.28 · Warp Stone 0.27 · Reactive Barrier 0.24 · Weapon
Shielding 0.23 · Torment Pulse 0.22 · Rusted Barrel 0.22 · Spirit Burn 0.22 ·
Restorative Shot 0.21 · Disarming Hex 0.20 · High-Velocity Rounds 0.20 ·
Spiritual Overflow 0.19 · Enchanter's Emblem 0.18 · Extra Stamina 0.18 ·
Extra Regen 0.18 · Mystic Vulnerability 0.17 · Superior Duration 0.15 ·
Escalating Exposure 0.15 · Opening Rounds 0.14 · Spirit Resilience 0.14 ·
Stamina Mastery 0.13 · Surge of Power 0.10 · Indomitable 0.10.

Ninguna se excluye por "no existir": todas están por debajo del umbral 30 % de
la ruta observada. Es una decisión de umbral, no de disponibilidad.

## Pruebas existentes

14 paquetes con pruebas, todos verdes. Cobertura relevante: `deadlock`
(BuildRoute, api con servidor local), `ui` (viewport, selección, scroll,
popups, terminales estrechas), `clean` (Wilson, filtros), `cache` (TTL,
stale-if-error).

## Riesgos y límites del diagnóstico

- La muestra de partidas es de 250 partidas por héroe: suficiente para la ruta
  observada, insuficiente para inferencia causal.
- `item-stats` y la muestra de partidas son poblaciones distintas; la UI ya
  advierte de ello (cambio preexistente en `detail.go`).
- No se validó visualmente la TUI en un terminal real; solo con pruebas.
- El WR por ítem no es evidencia causal de rendimiento.
