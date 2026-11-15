# Corrección de causa raíz: posiciones de habilidad en Deadlock

Fecha: 2026-11-07. Rama `master`. **Sin commits.**

---

## 1. Causa raíz

No era el algoritmo de mapeo. Era **una línea de cableado**.

`LoadBuilds` construía el índice `class_name → id de habilidad` iterando
`shop`:

```go
abilityByClass := make(map[string]int, len(shop))
for id, it := range shop {
    if it.ClassName != "" && it.Type == "ability" {
        abilityByClass[it.ClassName] = id
    }
}
```

Pero `shop` contiene **solo ítems comprables** (`upgrade` y `weapon`): las
habilidades se filtran al construirlo. El mapa salía con **0 entradas** de
345 ítems.

Consecuencia en cadena:

1. `canonicalAbilityOrder` no resolvía ninguna de las cuatro firmas →
   devolvía `nil`.
2. `canonicalSlots` recibía `canonical == nil` → devolvía `nil`.
3. `buildSkillSequences` caía al **fallback por frecuencia de uso**.
4. Los slots se asignaban por popularidad, no por posición real.

Y ocurría en **todos los héroes**, no solo en los cinco reportados. El
código del fallback estaba bien escrito; el problema es que se activaba
siempre.

Medición del fallo:

```
shop tiene 345 ítems
abilityByClass construido desde shop: 0 entradas
abilityClassIndex() real:             437 entradas
canonicalAbilityOrder(shop) = []
canonicalAbilityOrder(real) = [4011110259 4180486641 2424652896 1389230689]
```

El índice correcto ya existía (`abilityClassIndex`, que lee el catálogo
completo). Simplemente no se usaba.

### Corrección

```go
abilityByClass := a.abilityClassIndex()
```

Una línea. Los cinco héroes reportados pasaron de invertidos a exactos.

---

## 2. Por qué `signature1..4` no garantizaba el orden

Los metadatos **sí** son correctos y completos: 437 habilidades con
`class_name`, y los 65 héroes tienen las cuatro firmas. El fallo nunca
estuvo en el dato.

Fallaba la resolución: el índice contra el que se resolvían estaba vacío.
`canonicalAbilityOrder` hacía lo correcto (devolver `nil` en vez de
inventar) y el fallback hacía lo correcto (ordenar por uso), pero nadie
comprobaba que el fallback se estuviera usando cuando no tocaba.

---

## 3. Fixture independiente

`internal/deadlock/skill_slots_fixture_test.go` — las posiciones están
escritas a mano desde las referencias del juego (build 6860), no derivadas
del código bajo prueba:

| Héroe | 1 | 2 | 3 | 4 |
|---|---|---|---|---|
| Solomon | Setup Bishop | Kingside Knight | Rook Battery | Develop Queen |
| Paradox | Pulse Grenade | Time Wall | Kinetic Carbine | Paradoxical Swap |
| Venator | Consecrating Grenade | Gutshot | Hex-Lined Snap Trap | Ira Domini |
| Violet | Living Ink | Flow State | Still Life | Abstract Landscape |
| Deadman Danny | Strix Salvo | Chimera Mk1 | Jettison Fuel | Close The Deal |

Dos pruebas sobre él:

- `TestSkillSlotsKnownHeroes` — el modelo produce exactamente esas
  posiciones, y `Rules.CanonicalOrder` es `true`.
- `TestSkillSlotsFixtureMatchesMetadata` — el fixture coincide con
  `heroes.json → items.signature1..4`. Si el juego cambia un kit, salta
  aquí antes que en la UI.

Y en `internal/ui/skill_slots_ui_test.go`, contra la **salida final** de la
TUI:

- `TestUISkillSlotsMatchFixture` — la leyenda muestra `1 <nombre>` … `4
  <nombre>` con los nombres correctos, y no avisa de fallback.
- `TestUIMaxLineMatchesSlots` — el texto `max 1 > 2 > …` coincide con
  `SkillOrder.Priority` y cada slot tiene nombre.
- `TestUIFallbackIsLabelled` — cuando el orden no es canónico, la vista lo
  dice.

---

## 4. Cobertura: todos los héroes jugables

`TestAuditAllHeroSlots` comprueba cada héroe con muestra en caché contra
los metadatos canónicos:

```
héroes con datos: 44 · slots incorrectos: 0 · fallback declarado: 1
sin muestra: 19 · sin metadatos: 2
```

**44 héroes verificados, 0 fallos.**

### El caso que no se puede resolver: Silver

Silver tiene **7 habilidades** en `ability-order-stats` (Tail Whack,
Mauling Leap, Go For The Throat, más las cuatro firmas), pero solo 4
firmas canónicas. `canonicalSlots` exige que el conjunto observado
coincida exactamente con el canónico; no coincide, así que **no puede
mapear** y cae al fallback.

Eso es correcto: inventar una posición para las tres habilidades extra
sería peor. Lo que faltaba era **declararlo**.

### Corrección añadida: el fallback ya no se disfraza de canónico

- `model.SkillRules.CanonicalOrder bool` — verdadero solo si los slots
  salen de `signature1..4`.
- `model.CleanBuild.CanonicalSkillOrder` — propagado por `clean.Build`.
- La leyenda de la TUI antepone **«orden por uso, no canónico»** cuando el
  flag es falso.

Así, un héroe con metadatos insuficientes no presenta la aproximación
estadística como si fuera la posición real del juego.

---

## 5. Secuencia estadística intacta

No se tocó `bestSkillOrder` ni `PriorityFromSequence`. La secuencia de
mejoras sigue saliendo de la partida más jugada; lo que cambió es **a qué
slot se asigna cada identificador estable**. La prioridad `max …` se
deriva de la secuencia ya remapeada, así que ahora el texto corresponde a
los slots reales.

Verificado en `TestUIMaxLineMatchesSlots`: la prioridad del modelo aparece
literal en la vista y cada slot tiene nombre.

---

## 6. Resultado

Antes / después en la pestaña de habilidades:

| Héroe | Antes (slot 1) | Ahora (slot 1) |
|---|---|---|
| Solomon | Develop Queen | **Setup Bishop** |
| Paradox | Kinetic Carbine | **Pulse Grenade** |
| Venator | Ira Domini | **Consecrating Grenade** |
| Violet | Living Ink ✓ | Living Ink |
| Deadman Danny | Chimera Mk1 | **Strix Salvo** |

Violet acertaba el slot 1 por casualidad (Living Ink es la más subida);
sus slots 2-4 estaban mal.

---

## 7. Pruebas ejecutadas

```
go test ./...      verde en los 14 paquetes con pruebas
go vet ./...       limpio
gofmt -l           limpio
git diff --check   limpio
LoL                lolalytics, ugg, ddragon, clean, model: verde
```

Pruebas nuevas: 6 (2 de fixture en `deadlock`, 3 de UI, 1 de auditoría
masiva).

Builds de objetos: intactas. `TestIntegrationFourHeroes` y las pruebas de
coste siguen verdes.

---

## 8. Archivos modificados

| Archivo | Cambio |
|---|---|
| `internal/deadlock/api.go` | índice de habilidades desde el catálogo completo; flag `CanonicalOrder` |
| `internal/model/model.go` | `SkillRules.CanonicalOrder`, `CleanBuild.CanonicalSkillOrder` |
| `internal/clean/clean.go` | propaga el flag |
| `internal/ui/detail.go` | aviso «orden por uso, no canónico» |
| `internal/deadlock/skill_slots_fixture_test.go` | nuevo: fixture independiente |
| `internal/deadlock/audit_all_heroes_test.go` | nuevo: 44 héroes |
| `internal/ui/skill_slots_ui_test.go` | nuevo: salida final de la TUI |

---

## 9. Limitaciones

1. **Silver queda en fallback declarado.** Sus 7 habilidades observadas no
   se pueden mapear a 4 firmas sin más información. No se inventó nada.
2. **19 héroes sin muestra en caché** no se pudieron auditar. La prueba se
   salta, no falla.
3. **2 héroes sin metadatos completos** (sin las cuatro firmas).
4. **No hay validación visual real.** Todo son pruebas y renders en texto.
   No se declara validación visual.

---

## 10. Propuesta de commits (sin ejecutar)

```
1. fix(deadlock): resolver habilidades desde el catálogo completo
   internal/deadlock/api.go

2. feat(model): marcar si el orden de slots es canónico
   internal/model/model.go
   internal/clean/clean.go
   internal/ui/detail.go

3. test: fixture independiente de posiciones y auditoría de 44 héroes
   internal/deadlock/skill_slots_fixture_test.go
   internal/deadlock/audit_all_heroes_test.go
   internal/ui/skill_slots_ui_test.go
```
