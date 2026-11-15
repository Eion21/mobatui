# Corrección definitiva de habilidades y mejoras de TUI — Deadlock

Fecha: 2026-11-14. Rama `master`. **Sin commits.**
Datos: caché local. Cero tráfico HTTP nuevo, cero dependencias nuevas.

---

## 1. Resumen ejecutivo

La causa raíz de las habilidades mal posicionadas era **una línea de
cableado**: el índice `class_name → habilidad` se construía desde `shop`,
que solo contiene objetos comprables. Salía vacío y *todos* los héroes caían
al fallback por frecuencia.

Corregido eso, quedaba **Silver**: siete registros estadísticos para cuatro
teclas. La relación entre forma humana y transformada está en los metadatos
del juego (`dependent_abilities` + flag `DisplayAsSubAbility`), no en los
nombres. Agrupando por esa relación, Silver muestra cuatro espacios sin
inventar nada.

Se completaron además cinco mejoras de interfaz: cabecera que se compacta al
desplazarse, situacionales agrupados por función, ampliaciones en orden
cronológico, tarjetas compactas en terminales bajas y nombres de habilidad
sin truncar cuando hay espacio.

**Resultado:** 44 héroes con muestra, **todos** con orden canónico, 0
fallback. Suite completa verde. LoL intacto.

---

## 2. Causa raíz y verificación del cableado

### El fallo

```go
// ANTES (roto): shop solo trae upgrade/weapon
abilityByClass := make(map[string]int, len(shop))
for id, it := range shop {
    if it.ClassName != "" && it.Type == "ability" { ... }
}
// → 0 entradas de 345 ítems
```

```go
// AHORA: catálogo completo (437 habilidades)
abilityByClass := a.abilityClassIndex()
```

### Verificación

`TestWiringIndexComesFromFullCatalog` comprueba las dos mitades:

1. `shop` contiene **0** habilidades (precondición).
2. `abilityClassIndex()` tiene **≥100** entradas y resuelve las cuatro firmas
   de Solomon.

Si alguien vuelve a derivar el índice de `shop`, la prueba falla en el
segundo punto.

`TestWiringLoadBuildsResolvesCanonical` ejecuta el flujo real de `LoadBuilds`
y cuenta: **44 canónicos, 0 fallback**.

### Cadena auditada

`LoadBuilds` → `abilityClassIndex` + `abilityItemsByClass` →
`abilitySlotMap` → `buildSkillSequences` → `model.SkillRules` →
`clean.Build` → `CleanBuild.CanonicalSkillOrder` → `detail.abilityGrid`.

La frecuencia de uso **solo** se usa cuando no hay mapeo canónico, y en ese
caso se declara (`CanonicalOrder: false`).

---

## 3. Silver y las transformaciones

### Metadatos, no nombres

Los siete ids de Silver, con su relación declarada por el juego:

| id | nombre | class_name | relación |
|---|---|---|---|
| 1385849824 | Slam Fire | `ability_werewolf_unloadgun` | signature1 |
| 2565763509 | Go For The Throat | `ability_werewolf_frenzy` | dependiente de Slam Fire |
| 2717130900 | Boot Kick | `ability_werewolf_kickflip` | signature2 |
| 4009459244 | Mauling Leap | `ability_werewolf_maulingleap` | dependiente de Boot Kick |
| 3983754897 | Entangling Bola | `ability_werewolf_netshot` | signature3 |
| 3117047651 | Tail Whack | `ability_werewolf_cripplingslash` | dependiente de Entangling Bola |
| 515791019 | Lycan Curse | `ability_werewolf_transformation` | signature4 |

La relación sale de `dependent_abilities` con flag `DisplayAsSubAbility`. No
hay ninguna comprobación por nombre.

### Resultado

```
1 Slam Fire       ◆ ◆ · · ◆ · · · · · · ◆ · · ·
2 Boot Kick       · · ◆ · · ◆ · · ◆ · · · ◆ · ·
3 Entangling Bola · · · ◆ · · · · · · · · · ◆ ◆
4 Lycan Curse     · · · · · · ◆ ◆ · ◆ ◆ · · · ·
```

**Cuatro filas, no siete.** `max 4 > 1 > 2 > 3` sobre las teclas reales.

### Sin doble conteo

Los topes por espacio son el **máximo** observado, no la suma: dos formas del
mismo espacio no pueden dar tope 8. `TestSilverNoDoubleCounting` lo verifica.

### Otros héroes con transformaciones

Barrido completo del catálogo: **solo Silver** tiene sub-habilidades con
`DisplayAsSubAbility`, y solo Silver supera los 4 registros. La solución es
genérica (agrupa por la relación declarada), no una excepción para Silver.

---

## 4. Cobertura real de héroes

`TestCoverageAudit`, sobre la caché local:

| Métrica | Valor |
|---|---|
| Héroes en el catálogo | **65** |
| Con las 4 firmas resolubles | **63** |
| Con muestra estadística | **44** |
| → con orden canónico | **44** |
| → con fallback declarado | **0** |
| Con habilidades de cambio de forma | **1** (Silver) |
| Sin muestra | **21** |
| Sin las 4 firmas | **2** (Fathom, hero_testhero) |

### Por qué las cifras anteriores no cuadraban

- **65** es el catálogo completo de `/v1/assets/heroes`.
- **63** tienen las cuatro firmas resolubles; Fathom y `hero_testhero` no
  (héroes en desarrollo sin kit publicado).
- **44** tienen muestra en `ability-order-stats` cacheada.
- **21** no tienen muestra: **no es falta de posiciones**, es falta de
  estadísticas. Sus posiciones canónicas existen; simplemente no hay datos
  de orden que mostrar.

La distinción importa: ausencia de estadísticas ≠ ausencia de posiciones.

---

## 5. Comportamiento canónico y fallback

### Canónico (44 héroes)

- Cuatro filas, teclas 1-4.
- Nombres de la firma canónica.
- Matriz de progreso estadístico intacta.
- `max` sobre las teclas reales.
- Sin aviso.

### Fallback (0 héroes hoy, pero implementado)

- Referencias neutrales **A, B, C…** en vez de números.
- Aviso **arriba del panel**: `Orden por uso · posiciones no verificadas`.
- `max A > B > D > C`, nunca `max 5 > 6 > 7`.
- Partidas y estadísticas siguen visibles.

`TestSlotLabelFallbackNeutral`, `TestFallbackShowsWarningAbove` y
`TestCanonicalNoWarning` cubren ambos caminos.

---

## 6. Cambios visuales implementados

### 4.1 Cabecera compacta al desplazarse

- Arriba: cabecera completa (retrato, título, parche, tendencia).
- Al desplazarse: una línea con nombre y parche.
- Al volver arriba: se recupera.
- El umbral ya no es fijo en 20 filas: se calcula con el alto real de la
  cabecera (`headerFits`). Con retrato son ~10 líneas, así que a 20 filas no
  dejaba viewport.
- Sin saltos de scroll ni cambios de selección: `View` re-layouta pero no
  toca `YOffset`.

7 pruebas: expansión, compactación, restauración, no-salto, selección
estable, terminal baja, resize.

### 4.2 Situacionales agrupados por función

Seis grupos en orden fijo: Anticuración, Defensa contra balas, Defensa
espiritual, Resistencia al control, Supervivencia, Daño adicional.

Reutiliza la etiqueta que ya calcula `situationalFunction`. Sin algoritmo
nuevo. Objetos sin función reconocida se listan al final sin categoría
inventada. Sin duplicados.

5 pruebas: agrupación, no duplicados, orden estable, selección, terminal
angosta.

### 4.3 Ampliaciones en orden cronológico

Antes: orden de score (~29, ~20, ~33, ~22 min). Ahora: minuto ascendente,
score como desempate.

```
Warp Stone          ~20 min
Torment Pulse       ~22 min
Spiritual Overflow  ~29 min
Spirit Burn         ~33 min
```

2 pruebas: orden y determinismo.

### 4.4 Tarjetas compactas

| Modo | Alto | Contenido |
|---|---|---|
| Normal | 13 líneas | icono + nombre (2) + relación (2) + coste |
| Compacta | **5 líneas** | nombre + relación + coste, una línea cada uno |

Conserva nombre, número de paso, mejora, coste, minuto y confianza. Se activa
con viewport < 28 filas. Navegación y popups intactos.

6 pruebas: altura, datos conservados, relación, navegación, no desborde,
determinismo.

### 4.5 Nombres de habilidad sin truncar

`labelW` era fijo en 18. Ahora `skillLabelWidth` mide el nombre más largo y
solo recorta si la matriz de niveles no cabría.

`Abstract Landscape` (18 caracteres) ya se ve completo a 120 y 140 columnas.

5 pruebas: sin truncar en ancho, truncado controlado en estrecho, ancho
adaptativo, tope, alineación de la matriz.

---

## 7. Comparación antes/después

| Aspecto | Antes | Ahora |
|---|---|---|
| Slots de habilidad | Por frecuencia (todos los héroes) | Canónicos (44/44) |
| Solomon tecla 1 | Develop Queen | **Setup Bishop** |
| Silver | 7 filas + aviso de fallback | **4 filas**, sin aviso |
| Fallback | Números 1..N como teclas | Letras A/B/C + aviso arriba |
| Cabecera al scrollear | Completa (10 líneas) | Compacta (1 línea) |
| Situacionales | Lista mezclada | 6 grupos por función |
| Ampliaciones | Orden por score | Orden cronológico |
| Tarjeta (viewport bajo) | 13 líneas | **5 líneas** |
| `Abstract Landscape` | Truncado | Completo si cabe |

---

## 8. Archivos modificados por fase

### Fase 1 — cableado
- `internal/deadlock/api.go` — `abilityByClass := a.abilityClassIndex()`,
  `apiItem.DependentAbilities`, `apiDependent.IsSubAbility`, `allItems`,
  `abilityItemsByClass`
- `internal/deadlock/wiring_test.go` (nuevo)

### Fase 2 — Silver
- `internal/deadlock/api.go` — `abilitySlotMap`, `buildSkillSequences`
  reescrita con dos caminos
- `internal/deadlock/silver_test.go` (nuevo)
- `internal/deadlock/coverage_audit_test.go` (nuevo)
- `internal/deadlock/phase1_skills_test.go`, `audit_all_heroes_test.go`,
  `skill_slots_fixture_test.go` — adaptadas a la nueva firma

### Fase 3 — fallback
- `internal/model/model.go` — `SkillRules.CanonicalOrder`,
  `CleanBuild.CanonicalSkillOrder`
- `internal/clean/clean.go` — propaga el flag
- `internal/ui/detail.go` — `slotLabel`, aviso arriba del panel
- `internal/ui/skills_presentation_test.go` (nuevo)

### Fase 4 — interfaz
- `internal/ui/detail.go` — `scrolled`, `headerFits`, `compactHeader`,
  `hybridSections` con grupos, `situationalOrder`, `emitIndices`,
  `emitFiltered`, `skillLabelWidth`
- `internal/ui/deadlock_card.go` — `dlRelation`, tarjeta compacta
- `internal/deadlock/hybrid.go` — orden cronológico de ampliaciones
- `internal/ui/header_compact_test.go` (nuevo)
- `internal/ui/situationals_group_test.go` (nuevo)
- `internal/ui/compact_card_test.go` (nuevo)
- `internal/ui/skill_names_test.go` (nuevo)
- `internal/deadlock/hybrid_test.go` — 2 pruebas de ampliaciones

---

## 9. Pruebas ejecutadas

```
go test ./...      verde en los 14 paquetes con pruebas
go vet ./...       limpio
gofmt -l .         limpio
git diff --check   limpio
LoL                lolalytics, ugg, ddragon, clean, model: verde
```

Pruebas nuevas en esta entrega: **35**

| Área | N |
|---|---|
| Cableado | 2 |
| Silver | 5 |
| Cobertura | 1 |
| Presentación skills | 5 |
| Cabecera compacta | 7 |
| Situacionales | 5 |
| Tarjeta compacta | 6 |
| Nombres de habilidad | 5 |
| Ampliaciones | 2 |

---

## 10. Regresiones detectadas y corregidas

1. **Firma de `buildSkillSequences`** cambió de `[]int64` a `map[int64]int`.
   Cuatro pruebas existentes dejaron de compilar; actualizadas.
2. **`loadHeroBuilds`** ganó un parámetro (`itemsByClass`). Tres llamadores de
   prueba actualizados.
3. **El slot 3 de Silver** mostraba "Tail Whack" en vez de "Entangling Bola":
   el principal se elegía por id menor. Corregido pasando `primaries` desde la
   firma.
4. **Umbral fijo de 20 filas** para la cabecera compacta desbordaba a 120×20
   (22 líneas). Sustituido por `headerFits`, que mide la cabecera real.
5. **Una prueba de agrupación** fallaba por scroll insuficiente, no por el
   agrupador. Reescrita para comprobar títulos en vez de recorrer el viewport.

---

## 11. Limitaciones pendientes

1. **No hay validación visual en terminal real.** Los renders son capturas de
   texto a tamaños fijos, no la TUI ejecutándose. No se declara validación
   visual.
2. **21 héroes sin muestra** no se pudieron auditar estadísticamente. Sus
   posiciones canónicas existen; faltan datos de orden.
3. **2 héroes sin las cuatro firmas** (Fathom, hero_testhero): en desarrollo,
   sin kit publicado.
4. **El aviso de fallback no se ve en producción hoy** porque los 44 héroes
   con muestra son canónicos. Está implementado y probado con fixtures.
5. **El modo compacto de tarjetas** se activa por altura del viewport, no es
   seleccionable por el usuario. No se añadió configuración permanente.

---

## 12. Propuesta de commits separados (sin ejecutar)

```
1. fix(deadlock): resolver habilidades desde el catálogo completo
   internal/deadlock/api.go
   internal/deadlock/wiring_test.go

2. feat(deadlock): agrupar sub-habilidades por espacio canónico (Silver)
   internal/deadlock/api.go
   internal/deadlock/silver_test.go
   internal/deadlock/coverage_audit_test.go
   internal/deadlock/phase1_skills_test.go
   internal/deadlock/audit_all_heroes_test.go
   internal/deadlock/skill_slots_fixture_test.go

3. feat(ui): fallback neutral y aviso de posiciones no verificadas
   internal/model/model.go
   internal/clean/clean.go
   internal/ui/detail.go
   internal/ui/skills_presentation_test.go

4. feat(ui): cabecera compacta al desplazarse
   internal/ui/detail.go
   internal/ui/header_compact_test.go

5. feat(ui): situacionales agrupados por función
   internal/ui/detail.go
   internal/ui/situationals_group_test.go

6. fix(deadlock): ampliaciones en orden cronológico
   internal/deadlock/hybrid.go
   internal/deadlock/hybrid_test.go

7. feat(ui): tarjetas compactas y nombres de habilidad sin truncar
   internal/ui/deadlock_card.go
   internal/ui/detail.go
   internal/ui/compact_card_test.go
   internal/ui/skill_names_test.go
```

Los cambios preexistentes del working tree siguen sin commitear y deben
revisarse por separado.
