# Informe final — Rediseño del generador de builds de Deadlock

Fecha: 2026-11-07. Rama `master`. Sin commits (por instrucción).
Todo el análisis y las pruebas se hicieron **offline** sobre `data/cache/`.
Cero solicitudes HTTP nuevas contra la API pública.

---

## 1. Resumen ejecutivo

El generador anterior no estaba roto: estaba **midiendo lo que no importa**.
Producía una lista de compras frecuentes y la presentaba como build. De ahí
los tres síntomas del reporte: pocos objetos finales, cadenas cortadas y
compras relevantes relegadas a «Fuera de la ruta».

Se construyó, en cinco fases, un pipeline nuevo que convive con el anterior:

| Fase | Entrega | Estado |
|---|---|---|
| 0 | Diagnóstico reproducible con arnés offline | Cerrada |
| 1 | Mapeo canónico de habilidades + numeración continua | Cerrada |
| 2 | Motor de progresión (`internal/progression`) | Cerrada |
| 3 | Evaluador estadístico con Beta-Binomial | Cerrada |
| 4 | Constructor híbrido con diagnósticos | Cerrada |
| 5 | Rendimiento: 41.8× en consulta repetida | Cerrada |

**El generador anterior no se eliminó.** Wilson sigue en su sitio y la ruta
observada se sigue calculando igual. El nuevo resultado es experimental y
comparable.

---

## 2. Diagnóstico de causas raíz

### 2.1 Hueco de numeración 7 → 9

`cardItem.order` era el índice de la ruta de consenso, no un número de paso.
`dlBuildCards` cortaba a `perPhase = 8` tarjetas por fase y `filterUpgrades`
podía quitar más. Los supervivientes conservaban su `order` original.

Evidencia: Healbane tenía `ruta#8` y quedaba en el **puesto 12 de 18** por
score en MID, por debajo del corte. Su `rel = 0.24` competía contra
Healing Booster (0.70), que no pertenece a la ruta.

### 2.2 Transcendent Cooldown fuera de la ruta

Mismo mecanismo: `ruta#17`, `rel = 0.30`, **puesto 10 de 17** en LATE.
Cortado por `perPhase = 8`.

### 2.3 Pocos objetos finales

El corte era por *tarjeta*, no por *objeto final*. Una cadena de tres
(Extra Spirit → Improved Spirit → Boundless Spirit) consume tres tarjetas
para producir un objeto. Con 15 tarjetas se obtenían 8 objetos.

### 2.4 Habilidades de Solomon invertidas

`buildSkillSequences` asignaba slots **por frecuencia de uso**, no por el
orden canónico del héroe. Con los datos reales (878 filas):

| slot asignado | habilidad | firma canónica |
|---|---|---|
| 1 | Rook Battery | signature3 |
| 2 | Develop Queen | signature4 |
| 3 | Setup Bishop | signature1 |
| 4 | Kingside Knight | signature2 |

Exactamente la inversión reportada. El orden canónico sí existe en el dato:
`heroes.json` → `items.signature1..4`.

### 2.5 Esperas repetidas

`FetchRuns` llamaba a `shopItems()` en cada consulta, recargando y
re-decodificando los 782 ítems del catálogo. Ese era el **90 %** del tiempo
de una consulta repetida.

---

## 3. Archivos modificados por fase

### Fase 0 (diagnóstico, solo pruebas nuevas)
- `internal/deadlock/diag_baseline_test.go` (nuevo)
- `internal/ui/diag_cards_test.go` (nuevo)
- `.perch/plans/fase0-diagnostico.md` (nuevo)

### Fase 1 (habilidades y numeración)
- `internal/deadlock/api.go` — `apiHero.Items`, `canonicalAbilityOrder`,
  `canonicalSlots`, `shopItemsFull`, `abilityClassIndex`
- `internal/ui/detail.go` — campo `step`, `numberSteps`
- `internal/ui/deadlock_card.go` — usa `step` en tarjeta y selección
- `internal/deadlock/phase1_skills_test.go` (nuevo)
- `internal/ui/phase1_regression_test.go` (nuevo)
- `internal/ui/route_loading_test.go` — assertion por identidad, no por número

### Fase 2 (motor de progresión, paquete nuevo)
- `internal/progression/catalog.go` (nuevo)
- `internal/progression/simulate.go` (nuevo)
- `internal/progression/build.go` (nuevo)
- `internal/progression/progression_test.go` (nuevo)

### Fase 3 (evaluador)
- `internal/deadlock/evaluate.go` (nuevo)
- `internal/deadlock/evaluate_test.go` (nuevo)

### Fase 4 (constructor híbrido)
- `internal/deadlock/hybrid.go` (nuevo)
- `internal/deadlock/hybrid_test.go` (nuevo)

### Fase 5 (rendimiento)
- `internal/deadlock/runs.go` — `runsMemo`, `runsCatalog`, `perfHook`
- `internal/deadlock/api.go` — campos `runsCat`/`runsComps`
- `internal/deadlock/perf_test.go` (nuevo)
- `internal/deadlock/perf_helper_test.go` (nuevo)
- `internal/deadlock/overfit_test.go` (nuevo)

### Cambios preexistentes preservados (no míos)
`README.md`, `internal/ui/app.go`, `internal/ui/route.go`, `main.go`,
`internal/ui/ui_test.go`, `internal/ui/viewport_test.go`,
`internal/ui/route_semantics_test.go`, `main_test.go`, y el snapshot
estático en `api.go`. Se conservaron íntegros.

---

## 4. Arquitectura implementada

```
internal/progression/     dominio puro, sin TUI ni red
  catalog.go   catálogo validado: cadenas, ciclos, costes incrementales
  simulate.go  simulación de inventario con capacidad y consumo
  build.go     adaptador desde el catálogo crudo de Deadlock

internal/deadlock/
  runs.go      (existente) ruta observada + memoización acotada
  evaluate.go  (nuevo) puntuación por candidato con incertidumbre
  hybrid.go    (nuevo) constructor: ruta + ampliaciones + situacionales
```

Separación de responsabilidades:
- `progression` **garantiza validez**: nunca inventa objetos, rechaza ciclos,
  respeta capacidad, consume componentes.
- `evaluate` **puntúa**: popularidad, rendimiento ajustado, muestra,
  incertidumbre, consistencia.
- `hybrid` **decide**: qué entra, en qué orden, y por qué.

---

## 5. Pruebas realizadas y resultados

`go test ./...` — **verde en los 14 paquetes con pruebas**.
`go vet ./...` limpio. `gofmt -l internal/` limpio. `git diff --check` limpio.

Pruebas nuevas por área:

| Área | Pruebas | Qué cubren |
|---|---|---|
| Progresión | 20 | cadenas, bifurcaciones, ciclos, capacidad, consumo, costes, determinismo |
| Evaluador | 13 | popularidad vs WR, muestra pequeña, denominador por fase, encogimiento bayesiano, consistencia |
| Híbrido | 12 | cadenas completas, umbral, final vs componente, orden temporal, determinismo, confianza |
| Habilidades | 7 | orden canónico, fallback por uso, kit incompleto, topes |
| Numeración | 4 | continuidad, vacío, fuera de ruta, plegado de mejoras |
| Rendimiento | 5 | segunda consulta barata, mismo resultado, invalidez, tope, claves |
| Sobreajuste | 3 | Violet, Pocket, Celeste con las mismas invariantes |

**Regresión de LoL**: `lolalytics`, `ugg`, `ddragon`, `clean`, `model` y las
pruebas de UI de LoL pasan sin cambios. No se tocó ningún algoritmo de LoL.

---

## 6. Comparación de builds de Violet

Muestra: 250 partidas. Umbral 30 % conservado.

### Ruta observada (anterior)
- 17 pasos en la ruta
- **15 tarjetas visibles** (Healbane y Transcendent Cooldown cortados)
- **8 objetos finales**
- Hueco de numeración 7 → 9

### Build híbrida (nueva)
- 15 entradas, ordenadas por minuto de compra
- **9 objetos finales**: Spirit Shielding, Mystic Shot, Rapid Recharge,
  Spirit Lifesteal, Transcendent Cooldown, Tankbuster, Boundless Spirit,
  Healbane, Radiant Regeneration
- **0 rechazos**, 39 200 almas, confianza global 0.61 (moderada)
- Numeración continua 1..15

Mejoras concretas:
- **Transcendent Cooldown entra en la ruta** (antes solo en «Fuera de la ruta»).
- **Boundless Spirit completa su cadena de 3** dentro de la ruta.
- **Healbane ya no se pierde** por el corte de fase.
- Cada entrada declara su cadena, su confianza y su procedencia.

### Ampliaciones (4)
Objetos válidos que no entraron por umbral o por capacidad, para partidas
largas. Incluye Spiritual Overflow y Superior Cooldown.

### Situacionales (14)
Etiquetados por función verificable: Anticuración, Defensa contra balas,
Defensa espiritual, Resistencia al control, Supervivencia, Daño adicional.
Ninguna etiqueta afirma contrarrestar a un héroe concreto.

---

## 7. Estado de las habilidades de Solomon

**Corregido y verificado con datos reales.**

| slot | habilidad | firma |
|---|---|---|
| 1 | Setup Bishop | signature1 |
| 2 | Kingside Knight | signature2 |
| 3 | Rook Battery | signature3 |
| 4 | Develop Queen | signature4 |

El slot ahora sale de `heroes.json → items.signature1..4`, resuelto contra
el catálogo por `class_name`. Si el kit está incompleto o una clase no se
resuelve, se cae al orden por uso (comportamiento histórico) en vez de
inventar un mapeo.

Verificado también en Violet, Pocket y Celeste mediante las pruebas de
sobreajuste.

---

## 8. Comparación de rendimiento

Medido sobre caché local, sin red:

| Etapa | Antes | Después |
|---|---|---|
| `FetchRuns` (1.ª vez) | 30.0 ms | 30.0 ms |
| `FetchRuns` (2.ª vez) | 29.6 ms | **0.64 ms** |
| `BuildRoute` (anterior) | 1.4 ms | 1.4 ms |
| `Evaluate` (nuevo) | — | 0.7 ms |
| `BuildHybrid` (nuevo) | — | 0.86 ms |
| `LoadBuilds` (44 héroes) | 334 ms | 311 ms |

**41.8× más rápido** al volver a un héroe ya consultado.

Causa: `shopItems()` se llamaba en cada `FetchRuns` y re-decodificaba 782
ítems. Ahora el catálogo se resuelve una vez por cliente (`runsCatalog`) y
las runs se memorizan con clave que incluye la fecha de los datos.

**La memoización no sirve datos viejos**: la clave contiene el `fetchedAt`
de la caché de disco, así que un refresco real la invalida. Está acotada a
8 héroes.

---

## 9. Tráfico HTTP efectivo

**Cero solicitudes nuevas.** Todo el trabajo se hizo con `cache.Offline =
true` sobre las respuestas ya presentes en `data/cache/`. No se aumentó
concurrencia, reintentos, timeouts ni límites.

---

## 10. Riesgos pendientes

1. **La validación visual real sigue pendiente.** No se abrió la TUI en un
   terminal real. Todo se verificó con pruebas automatizadas. El brief es
   explícito: no declarar esto completado sin verlo.
2. **El constructor híbrido no está integrado en la TUI.** Existe como
   componente de dominio con pruebas; la presentación descrita en la Fase 5
   (secciones EARLY/MID/LATE/AMPLIACIONES/SITUACIONALES) no está cableada.
3. **La confianza global de Violet es 0.61 (moderada)**, no alta. Es honesto:
   la muestra de 250 partidas no sostiene más.
4. **El denominador por fase es una aproximación.** Se usa la última compra
   registrada como proxy de "la partida llegó a esa fase", porque la muestra
   no trae duración. Es mejor que dividir entre todas las partidas, pero no
   es exacto.
5. **Los cuantiles Beta son una aproximación normal**, declarada en el código.
   Suficiente para etiquetar incertidumbre; no para inferencia fina.
6. **El clasificador de situacionales es por nombre.** Es verificable y
   conservador, pero un objeto nuevo sin palabra clave queda sin etiqueta y
   no se propone.

---

## 11. Limitaciones de datos

- Muestra de 250 partidas por héroe: suficiente para la ruta observada,
  insuficiente para inferencia causal.
- `item-stats` y la muestra de partidas son poblaciones distintas. La UI ya
  advierte de ello.
- No hay datos de builds comunitarias verificables en la caché. La procedencia
  `community` está definida en el modelo pero **no se usa**: no se inventó
  ninguna fuente.
- No hay duración de partida, así que no hay denominador exacto por fase.
- El WR por ítem **no** es evidencia causal de rendimiento. El evaluador lo
  trata como señal ponderada por confianza, nunca como prueba.

---

## 12. Cambios recomendados para el futuro

1. **Integrar el híbrido en la TUI** con la presentación de la Fase 5, y
   mantener la ruta observada accesible para comparar.
2. **Validar visualmente** en terminal real: 80×24, 120×40, y una terminal
   estrecha. Es el paso que falta para cerrar la Fase 5 de verdad.
3. **Medir pesos con datos**, no fijarlos: hoy `score` usa 1.0/1.2/0.8
   documentados. Con más héroes se puede calibrar contra una partición
   independiente.
4. **Añadir duración de partida** a la muestra si la API la expone, para un
   denominador por fase exacto.
5. **Explorar beam search** solo si las mediciones lo justifican. Hoy el
   constructor es greedy determinista y produce rutas válidas; añadir búsqueda
   sin evidencia sería complejidad gratuita.
6. **Investigar fuentes comunitarias** con contrato verificable antes de usar
   la procedencia `community`.

---

## 13. Propuesta de commits separados (sin ejecutar)

```
1. test(deadlock): arnés de diagnóstico offline y línea base de Violet
   internal/deadlock/diag_baseline_test.go
   internal/ui/diag_cards_test.go

2. fix(deadlock): orden canónico de habilidades desde signature1..4
   internal/deadlock/api.go
   internal/deadlock/phase1_skills_test.go

3. fix(ui): numeración de pasos continua tras el filtrado
   internal/ui/detail.go
   internal/ui/deadlock_card.go
   internal/ui/phase1_regression_test.go
   internal/ui/route_loading_test.go

4. feat(progression): motor de progresión de objetos
   internal/progression/*

5. feat(deadlock): evaluador estadístico con Beta-Binomial
   internal/deadlock/evaluate.go
   internal/deadlock/evaluate_test.go

6. feat(deadlock): constructor híbrido con diagnósticos
   internal/deadlock/hybrid.go
   internal/deadlock/hybrid_test.go

7. perf(deadlock): reutilizar catálogo y runs entre consultas
   internal/deadlock/runs.go
   internal/deadlock/api.go
   internal/deadlock/perf_test.go
   internal/deadlock/perf_helper_test.go
   internal/deadlock/overfit_test.go

8. docs: diagnóstico de Fase 0 e informe final
   .perch/plans/fase0-diagnostico.md
```

Los cambios preexistentes del working tree (`README.md`, `app.go`,
`route.go`, `main.go`, tests de viewport) **no** entran en estos commits:
son trabajo previo del usuario y deben revisarse por separado.

---

## Criterio fundamental de éxito

El programa pasó de mostrar una lista de compras frecuentes a construir una
progresión verificable: cada compra declara su cadena, su coste incremental,
su confianza y su procedencia; el inventario se simula y se comprueba; las
cadenas se completan; y lo que no entra queda explicado con un motivo.

Lo que **no** se afirma: que el win rate del jugador vaya a mejorar. Para eso
haría falta evidencia experimental, y no la hay.
