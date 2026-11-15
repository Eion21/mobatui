# Auditoría final de integración híbrida de Deadlock

Fecha: 2026-11-07. Rama `master`. Sin commits.
Datos: caché local. Cero solicitudes HTTP nuevas, cero dependencias nuevas.

---

## 1. Costes y componentes

### Causa raíz: dos compras reales invisibles

La ruta de Violet tenía **17 compras reales** pero la UI mostraba **15
entradas**. `assembleSequence` emitía entrada solo para el objeto final de
cada cadena; los componentes intermedios se compraban y se pagaban pero no
aparecían en ninguna parte.

Consecuencias encadenadas:

1. **Etiquetas incoherentes.** Tankbuster decía «Mejora de Mystic Burst» y
   Superior Cooldown «Mejora de Compress Cooldown», pero ninguno de los dos
   componentes estaba visible. El jugador no podía saber de dónde salían.
2. **Total de almas que no cuadraba.** Los importes visibles sumaban 26.400;
   el plan reportaba 39.200.

### Causa raíz: el gasto sumaba precios de catálogo

`s.spent += it.Cost` sumaba el precio absoluto de cada mejora. Pero el
componente **ya se había pagado** en un paso anterior. Comprar Extra Spirit
(800) y luego Improved Spirit (1.600) cuesta 1.600 en oro real, no 2.400.

### Correcciones

**`internal/progression/simulate.go`** — el gasto acumula lo que se paga en
cada paso: el incremental si el componente está en el inventario, el precio
completo si no.

**`internal/deadlock/hybrid.go`** — `assembleSequence` emite una entrada por
cada compra real. Los componentes intermedios llevan `ConsumedBy` y una
razón «paso hacia X». Las entradas se ordenan por minuto.

### Resultado

| Métrica | Antes | Ahora |
|---|---|---|
| Entradas en la ruta | 15 | **17** |
| Compras invisibles | 2 | **0** |
| Etiquetas «Mejora de» con base ausente | 2 | **0** |
| Total reportado | 39.200 | **28.800** |
| Suma de importes visibles en la UI | 26.400 | **28.800** |

La diferencia de 26.400 que reportaste era exactamente los dos componentes
ocultos: Mystic Burst (800) + Compress Cooldown (1.600) = 2.400. Y 39.200 era
el doble conteo de los componentes ya pagados.

**Verificado: la UI y el plan coinciden al alma.** `TestAuditUICostMatchesPlan`
suma los importes que la TUI muestra y los compara con `Plan.Spent`: 28.800 =
28.800.

### Pruebas añadidas

| Prueba | Qué cubre |
|---|---|
| `TestSpentUsesIncrementalCost` | el gasto es el oro real, no la suma de catálogo |
| `TestSpentDirectPurchasePaysFullPrice` | sin componente se paga el precio completo |
| `TestSpentComponentPresentVsAbsent` | las dos vías cuestan lo mismo |
| `TestSpentChainOfThree` | cadena de tres = precio del objeto final |
| `TestIncrementalMultiComponent` | dos componentes: incremental contra la suma |
| `TestAuditUICostMatchesPlan` | UI y plan cuadran (integración) |

### Consistencia en todos los héroes

`TestIntegrationFourHeroes` verifica las mismas invariantes en Violet,
Solomon, Pocket y Celeste. Los cuatro pasan sin rechazos y con inventario
válido en cada paso.

**No se fuerza la compra de componentes.** Si el componente no está en el
inventario, el motor compra el objeto directamente y paga el precio completo:
`TestSpentDirectPurchasePaysFullPrice` lo cubre.

---

## 2. Transición entre rutas

Ambas cargas son concurrentes. Verificados los dos órdenes y los casos de
error con siete pruebas:

| Prueba | Escenario |
|---|---|
| `TestTransitionObservedThenHybrid` | ruta primero: se muestra como «Ruta observada»; la híbrida la reemplaza |
| `TestTransitionHybridThenObserved` | build primero: la ruta tardía **no la pisa**, solo se guarda |
| `TestTransitionLateResponseOtherHero` | respuesta tardía de otro héroe: el detalle activo no cambia |
| `TestTransitionSelectionSurvivesBothOrders` | selección válida en ambos órdenes |
| `TestTransitionErrorThenHybrid` | error de ruta no impide la build |
| `TestTransitionHybridErrorFallsBackToRoute` | build fallida: sigue la ruta observada |
| `TestTransitionNoScrollReset` | la llegada no reinicia el scroll |

**La ruta provisional está identificada.** Mientras la híbrida no llega, la
cabecera dice «Ruta observada · N partidas · aparición ≥30 %». Cuando llega,
dice «Build híbrida · N objetos finales · confianza X». No hay cambio
silencioso de clasificación.

**La interfaz no se bloquea**: la ruta se pinta en cuanto llega y la build la
sustituye cuando está lista.

---

## 3. Invalidación de caché

### Causa raíz: `Session.Hybrids` no tenía vigencia

Guardaba la build por héroe sin ninguna marca de procedencia. Un cambio de
parche o de filtros dejaba resultados obsoletos servidos como actuales.
Invalidar `runsMemo` **no** invalidaba las builds de sesión, como sospechabas.

### Corrección

`HybridBuildView.Key` identifica de qué datos salió la build. `Session.HybridKey`
es la clave vigente (parche + filtros). Tres puntos de control:

1. **`hybridCmdFor`**: si la build guardada tiene otra clave, se descarta y se
   recalcula.
2. **`hybridLoadedMsg`**: la build que llega se marca con la clave vigente.
3. **`hybridCards`**: una build con clave distinta **no se sirve**; se cae a la
   ruta observada mientras se recalcula.

### Pruebas

| Prueba | Qué cubre |
|---|---|
| `TestHybridReusedWhenKeyMatches` | misma clave: no se recalcula |
| `TestHybridInvalidatedOnPatchChange` | parche nuevo: recálculo |
| `TestHybridInvalidatedOnFilterChange` | filtros distintos: recálculo |
| `TestHybridInFlightNotDuplicated` | carga en curso: no se relanza |
| `TestHybridKeyStampedOnArrival` | la build se marca al llegar |
| `TestHybridStaleResultDiscarded` | build obsoleta no se sirve |
| `TestHybridKeyDiffersByPatchAndFilters` | la clave distingue ambos |

Además, `r` (recarga forzada) crea una sesión nueva, lo que descarta
`Hybrids` por completo.

---

## 4. Validación de interfaz

### Las cinco secciones

`TestHybridSectionsVisible` recorre la vista con la rueda y verifica el orden:
EARLY → MID → LATE → AMPLIACIONES → SITUACIONALES.

### Desborde de 40×16: causa y corrección mínima

**Causa:** la cabecera del detalle ocupaba **9 líneas** en una terminal de 16
filas (retrato, título, parche que se parte en cuatro líneas, tendencia). El
viewport tiene un mínimo de 6, así que el total daba 21.

**Corrección:** por debajo de 20 filas, `headerBlock` devuelve una cabecera de
una línea con nombre y parche. **Por encima de 20 filas no cambia nada.**

| Tamaño | Antes | Ahora |
|---|---|---|
| 40×16 | 21 líneas (desborde) | **13** |
| 60×20 | cabe | cabe (sin cambios) |
| 80×24 | cabe | cabe (sin cambios) |
| 120×40 | cabe | cabe (sin cambios) |

`TestHybridNarrowTerminal` ahora exige que el alto quepa en **todos** los
tamaños, sin excepciones.

### Validación visual real: NO realizada

**No se abrió la TUI en un terminal real.** Todo lo anterior son pruebas
automatizadas y renders en texto. No se declara validación visual.

---

## 5. Cierre

```
go test ./...        verde en los 14 paquetes con pruebas
go vet ./...         limpio
gofmt -l             limpio
git diff --check     limpio
LoL                  lolalytics, ugg, ddragon, clean, model: verde
```

### Resultado final de Violet

- 17 entradas en la ruta (antes 15)
- 9 objetos finales
- 4 ampliaciones (todas LATE), 15 situacionales
- 28.800 almas, coincidiendo UI y plan
- 0 rechazos, confianza 0.61 (moderada)
- Numeración continua 1..17

### Archivos modificados en esta auditoría

| Archivo | Cambio |
|---|---|
| `internal/progression/simulate.go` | gasto por coste incremental real |
| `internal/deadlock/hybrid.go` | entrada por cada compra real; `chainStepEntry` |
| `internal/ui/route.go` | `HybridBuildView.Key` |
| `internal/ui/app.go` | `Session.HybridKey`, `hybridKeyFor`, vigencia en `hybridCmdFor` |
| `internal/ui/detail.go` | vigencia en `hybridCards`; `compactHeader` |
| `internal/progression/progression_test.go` | +5 pruebas de coste |
| `internal/deadlock/audit_*_test.go` | diagnóstico (temporal) |
| `internal/ui/audit_*_test.go` | transición, invalidación, coste UI, desborde |
| `internal/ui/hybrid_integration_test.go` | clave en el helper; alto exigido en todos los tamaños |

### Limitaciones pendientes

1. **Sin validación visual real.** Es la única que no puedo cerrar sin
   ejecutar la TUI y mirarla.
2. **La clave de vigencia no incluye la muestra.** Un refresco de datos dentro
   de la misma sesión no cambia el parche, así que la build guardada se
   reutiliza. En la práctica la muestra se refresca al recargar (tecla `r`),
   que crea sesión nueva. Si en el futuro se añade refresco en caliente, la
   clave debe incorporar la fecha de los datos.
3. **El clasificador de situacionales sigue siendo por nombre.** Un objeto
   nuevo sin palabra clave queda sin etiqueta y no se propone.
4. **La procedencia `community` no se usa.** No hay fuente verificable.

### Propuesta de commits (sin ejecutar)

```
1. fix(progression): gasto acumulado por coste incremental real
   internal/progression/simulate.go
   internal/progression/progression_test.go

2. fix(deadlock): una entrada por cada compra real de la cadena
   internal/deadlock/hybrid.go

3. fix(ui): vigencia de las builds híbridas de sesión
   internal/ui/route.go
   internal/ui/app.go
   internal/ui/detail.go

4. fix(ui): cabecera compacta en terminales de menos de 20 filas
   internal/ui/detail.go

5. test: auditoría de costes, transición, invalidación y desborde
   internal/ui/audit_*_test.go
   internal/ui/hybrid_integration_test.go
```

Los cambios preexistentes del working tree siguen sin commitear y deben
revisarse por separado.
