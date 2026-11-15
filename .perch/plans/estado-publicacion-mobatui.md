# Estado y preparación de publicación — MOBA TUI

Fecha: 2026-11-14. Rama `master`. **Sin commits, sin push, sin tags.**
Todo lo verificado aquí se ejecutó; lo que no se pudo verificar se dice.

---

## 1. Estado inicial del proyecto

```
pwd                 /home/jervis/Proyectos/Desarrollo/Builds
branch              master
remotes             (ninguno configurado)
commits             2 (01028dc, 5164ced)
working tree        47 entradas: 12 modificadas, 35 sin seguimiento
diff                1391 inserciones, 130 borrados en 12 archivos
módulo              github.com/Eion21/builds-tui  (antes del renombrado)
```

El repositorio local tiene historia previa pero **no tiene remoto**: no hay
nada que sincronizar todavía. El remoto `github.com/Eion21/mobatui` existe
según indicaste, pero no está enlazado.

---

## 2. Qué de la instrucción anterior ya estaba hecho

Verificado ejecutando, no leyendo:

| Funcionalidad | Implementada | Probada | Evidencia |
|---|---|---|---|
| Cableado canónico de habilidades | Sí | Sí | `api.go:581` usa `abilityClassIndex()`; `TestWiring*` |
| Solomon y demás héroes | Sí | Sí | `TestSkillSlotsKnownHeroes` (5 héroes) |
| Silver y transformaciones | Sí | Sí | `TestSilver*` (5 pruebas), 4 filas |
| Fallback de habilidades | Sí | Sí | `TestSlotLabel*`, `TestFallbackShows*` |
| Cabecera compacta | Sí | Sí | `TestHeader*` (7 pruebas) |
| Situacionales agrupados | Sí | Sí | `TestSituationals*` (5 pruebas) |
| Orden de ampliaciones | Sí | Sí | `TestExtensions*` (2 pruebas) |
| Tarjetas compactas | Sí | Sí | `TestCompactCard*` (6 pruebas) |
| Nombres completos | Sí | Sí | `TestSkillNames*` (5 pruebas) |
| Generador híbrido | Sí | Sí | `TestHybrid*`, `TestIntegrationFourHeroes` |
| Invalidación de caché | Sí | Sí | `TestHybridInvalidated*`, `TestRunsMemo*` |
| Regresiones de LoL | Intacto | Sí | paquetes LoL verdes |

**Nada de las cinco fases quedaba pendiente.** No reimplementé ninguna.

Comprobé que está *conectado*, no solo presente:

- `main.go:160` implementa `LoadHybrid`.
- `app.go:311` cablea `HybridLoader` en la sesión.
- `detail.go:494` usa `hybridCards()` para construir las tarjetas.

---

## 3. Qué completé ahora

Solo la Etapa 3 (publicación), que no estaba hecha:

1. **Renombrado a MOBA TUI** — 116 imports, `go.mod`, `.goreleaser.yaml`,
   user-agents, `.gitignore`.
2. **Rutas de instalación global** — la caché escribía en `data/` relativo al
   directorio de trabajo. Un binario instalado con `go install` no corre en el
   repo: habría creado `data/` en cualquier sitio. Ahora usa
   `~/.cache/mobatui`.
3. **Nombre de configuración** — `builds/config.json` → `mobatui/config.json`.
4. **README reescrito** en inglés, honesto sobre limitaciones.
5. **`.perch/` excluido** del repositorio público (notas de trabajo internas).
6. **Verificación de secretos** — limpia.

---

## 4. Problemas encontrados

### 4.1 Caché relativa al CWD (real, corregido)

```go
dataDir = flag.String("data", "data", ...)   // antes
```

Instalado globalmente, `mobatui` habría escrito la caché en el directorio
desde el que se lanzara. Corregido con `defaultDataDir()` → `os.UserCacheDir()`.

Verificado: `./mobatui --help` desde `/tmp` muestra
`default "/home/jervis/.cache/mobatui"`.

### 4.2 Fixtures con ruta relativa (mitigado)

`testdata/` no se instala con `go install`. Añadí `defaultFixtureDir()`, que
busca junto al ejecutable y cae a la ruta relativa. Los fixtures son datos de
prueba: sin ellos la app funciona con caché y red.

### 4.3 User-agent con URL personal (corregido)

`internal/deadlock/api.go` apuntaba a `github.com/jervis/builds`, un usuario
distinto del dueño del repo. Corregido a `Eion21/mobatui`.

### 4.4 Sin remoto configurado (pendiente, informativo)

No hay `origin`. La Etapa 5 pide comprobar si el remoto tiene commits antes de
planificar la sincronización: **no se puede comprobar sin añadir el remoto**,
y eso queda fuera de lo autorizado.

---

## 5. Resultado de Silver

Verificado con datos reales en caché:

```
1 Slam Fire       ◆ ◆ · · ◆ · · · · · · ◆ · · ·
2 Boot Kick       · · ◆ · · ◆ · · ◆ · · · ◆ · ·
3 Entangling Bola · · · ◆ · · · · · · · · · ◆ ◆
4 Lycan Curse     · · · · · · ◆ ◆ · ◆ ◆ · · · ·
max 4 > 1 > 2 > 3
```

- **4 filas**, no 7.
- Las 3 formas transformadas (Go For The Throat, Mauling Leap, Tail Whack)
  comparten espacio con su principal, según `dependent_abilities` +
  `DisplayAsSubAbility`.
- Sin doble conteo: topes = máximo por espacio (4), no la suma.
- Sin aviso de fallback: el mapeo está demostrado.

---

## 6. Cobertura real de habilidades

`TestCoverageAudit`, ejecutado:

| Métrica | Valor |
|---|---|
| Héroes en el catálogo | 65 |
| Con las 4 firmas resolubles | 63 |
| Con muestra estadística | 44 |
| → con orden canónico | **44** |
| → con fallback declarado | **0** |
| Con cambio de forma | 1 (Silver) |
| Sin muestra | 21 |
| Sin las 4 firmas | 2 (Fathom, hero_testhero) |

**Ausencia de muestra ≠ ausencia de posiciones.** Los 21 sin muestra tienen
sus cuatro firmas canónicas; simplemente no hay datos de orden que mostrar.
Los 2 sin firmas están en desarrollo y no tienen kit publicado.

---

## 7. Mejoras visuales comprobadas

| Mejora | Medición |
|---|---|
| Cabecera compacta | 10 líneas → 1 al desplazarse; se restaura arriba |
| Situacionales | 6 grupos por función, orden fijo |
| Ampliaciones | 20, 22, 29, 33 min (cronológico) |
| Tarjetas compactas | 13 → 5 líneas por tarjeta |
| Nombres de habilidad | `Abstract Landscape` completo desde 120 columnas |

Alturas reales por tamaño (sin desborde):

| Tamaño | Alto | Límite |
|---|---|---|
| 120×40 | 37 | 40 |
| 80×24 | 21 | 24 |
| 60×20 | 20 | 20 |
| 40×16 | 13 | 16 |

---

## 8. Resultado de pruebas

```
go test ./...        verde en los 14 paquetes con pruebas
go vet ./...         limpio
gofmt -l .           limpio
git diff --check     limpio
go build -o /tmp/mobatui .   OK (14 MB)
```

**Prueba adicional de CI**: moví `data/` fuera y ejecuté la suite completa.
**Verde igual.** Eso confirma que CI pasará en un clon limpio, sin caché
personal.

`cmd/render lol` y `cmd/render deadlock` (los dos comandos que ejecuta CI)
salen con código 0.

Ningún test hace peticiones a las APIs públicas: usan `httptest` o
`cache.Offline = true`.

---

## 9. Estado de la documentación

`README.md` reescrito (8 KB), en inglés:

- Descripción, juegos, funcionalidades reales.
- Requisitos, instalación (`go install` y build local).
- Tabla de flags y de controles de teclado.
- Cómo funcionan las builds (4 pasos) y qué significa la confianza.
- Caché y frescura.
- **Limitaciones conocidas** — 7 puntos, incluyendo que no hay validación
  visual en terminal real y que las builds son consenso estadístico, no juego
  óptimo.
- Créditos, marcas y descargo de afiliación.
- Desarrollo, layout de paquetes, licencia.

**No hay capturas de imagen.** No generé ninguna: requeriría abrir la TUI real
y capturarla, y el brief prohíbe modificar permisos o el entorno gráfico para
forzarlo. El README no las promete.

---

## 10. Estado de licencia y recursos

- **LICENSE**: MIT, ya presente, `Copyright (c) 2026 Eion21`.
- **Dependencias**: todas MIT/BSD/Apache (Bubble Tea, Lip Gloss, bubbles,
  bubblezone, go-runewidth, gopsutil, x/image). Compatibles con MIT.
- **Datos de juego**: propiedad de Valve y Riot. El README lo declara
  explícitamente y descarga de afiliación.
- **Iconos e imágenes**: se descargan en runtime desde las APIs públicas, **no
  se versionan** en el repo. `data/` está en `.gitignore`.

**No asigné licencia nueva ni cambié la existente.** MIT ya estaba y es
compatible; confirmar los derechos sobre assets queda a tu criterio.

---

## 11. Estado de GitHub Actions

Ya existía y es correcto:

**`.github/workflows/ci.yml`**
- Matriz Ubuntu / macOS / Windows.
- `go-version-file: go.mod` (1.27.3).
- `permissions: contents: read` (mínimos).
- gofmt, `go vet`, `go test`, y los dos `cmd/render`.
- Sin llamadas a APIs públicas.

**`.github/workflows/release.yml`**
- Por tag `v*`, GoReleaser, `contents: write`.

**Ajuste necesario**: `.goreleaser.yaml` ya apunta a `mobatui` y a
`github.com/Eion21/mobatui`. Los binarios serán `mobatui` y `mobatui-render`.

---

## 12. Verificación de instalación

| Comprobación | Resultado |
|---|---|
| `go build -o /tmp/mobatui .` | OK, 14 MB |
| Ejecutar desde `/tmp` (fuera del repo) | OK, `--help` responde |
| Caché por defecto | `~/.cache/mobatui` (no `./data`) |
| Config por defecto | `~/.config/mobatui/config.json` |
| Suite sin `data/` | Verde |

**`go install github.com/Eion21/mobatui@latest` no se pudo probar**: el módulo
no está publicado todavía. No afirmo que funcione hasta que lo esté.

---

## 13. Riesgos de publicación

1. **El módulo aún no existe en el remoto.** `go install ...@latest` fallará
   hasta el primer push con un tag o al menos con el módulo publicado.
2. **Sin remoto configurado.** No sé si `Eion21/mobatui` ya tiene commits. Si
   los tiene, el primer push puede requerir merge, no fast-forward.
3. **macOS y Windows**: CI los compila, pero no hay prueba manual. El README lo
   dice.
4. **Parsing de Lolalytics** depende de su HTML. Si cambian el markup, LoL se
   rompe hasta actualizar el parser. Riesgo asumido y documentado.
5. **Sin capturas**: el README queda menos atractivo, pero no promete imágenes
   que no existen.
6. **`.perch/` excluido**: las notas de trabajo no se publican. Si querés
   conservarlas en el repo, hay que quitar la línea del `.gitignore`.

---

## 14. Propuesta de commits

Nueve commits, en orden de dependencia. **Ninguno ejecutado.**

```
1. fix(deadlock): resolver habilidades desde el catálogo completo
   internal/deadlock/api.go                       (abilityClassIndex)
   internal/deadlock/wiring_test.go

2. feat(deadlock): agrupar sub-habilidades por espacio canónico
   internal/deadlock/api.go                       (abilitySlotMap, buildSkillSequences)
   internal/deadlock/silver_test.go
   internal/deadlock/coverage_audit_test.go
   internal/deadlock/audit_all_heroes_test.go
   internal/deadlock/skill_slots_fixture_test.go
   internal/deadlock/phase1_skills_test.go

3. feat(model): marcar si el orden de slots es canónico
   internal/model/model.go
   internal/clean/clean.go
   internal/ui/detail.go                          (slotLabel, aviso)
   internal/ui/skills_presentation_test.go
   internal/ui/skill_slots_ui_test.go

4. feat(progression): motor de progresión de objetos
   internal/progression/                          (paquete completo)

5. feat(deadlock): evaluador estadístico y generador híbrido
   internal/deadlock/evaluate.go
   internal/deadlock/evaluate_test.go
   internal/deadlock/hybrid.go
   internal/deadlock/hybrid_test.go
   internal/deadlock/integration_heroes_test.go
   internal/deadlock/overfit_test.go

6. feat(ui): integrar la build híbrida en la TUI
   internal/ui/route.go
   internal/ui/app.go
   internal/ui/deadlock_card.go
   internal/ui/detail.go
   main.go
   internal/ui/hybrid_integration_test.go
   internal/ui/audit_*.go
   internal/ui/diag_cards_test.go
   internal/ui/render_violet_test.go
   internal/ui/route_loading_test.go
   internal/ui/route_semantics_test.go
   internal/ui/phase1_regression_test.go
   main_test.go

7. perf(deadlock): reutilizar catálogo y runs entre consultas
   internal/deadlock/runs.go
   internal/deadlock/perf_test.go
   internal/deadlock/perf_helper_test.go
   internal/deadlock/open_perf_test.go
   internal/deadlock/diag_baseline_test.go

8. feat(ui): mejoras de interfaz (cabecera, grupos, tarjetas, nombres)
   internal/ui/detail.go
   internal/ui/deadlock_card.go
   internal/deadlock/hybrid.go                    (orden de ampliaciones)
   internal/ui/header_compact_test.go
   internal/ui/situationals_group_test.go
   internal/ui/compact_card_test.go
   internal/ui/skill_names_test.go

9. chore: renombrar a MOBA TUI y preparar publicación
   go.mod
   main.go                                        (rutas de caché/fixtures)
   internal/config/config.go
   internal/deadlock/api.go                       (user-agent)
   internal/lolalytics/lolalytics.go              (user-agent)
   .goreleaser.yaml
   .gitignore
   README.md
   internal/ui/ui_test.go
   internal/ui/viewport_test.go
```

El commit 9 toca `api.go` y `main.go`, que también aparecen en commits
anteriores. Si preferís diffs estrictamente disjuntos, el renombrado puede ir
primero como commit 0 sobre el árbol actual antes de los demás. Es una
decisión de revisión, no técnica.

**No usar `git add .`**: arrastraría `data/` (ignorado, pero por si acaso) y
cualquier archivo temporal.

---

## 15. Pasos exactos para el primer push

Nada de esto está ejecutado. Requiere tu autorización.

```bash
# 1. Confirmar que el remoto está vacío o saber qué tiene
gh repo view Eion21/mobatui --json defaultBranchRef,isEmpty
# o: git ls-remote https://github.com/Eion21/mobatui

# 2. Enlazar el remoto
git remote add origin https://github.com/Eion21/mobatui

# 3. Revisar qué se va a subir (debe ser solo código, sin data/ ni .perch/)
git status --short
git check-ignore -v data/ .perch/

# 4. Crear los commits de la sección 14, uno por uno
#    (git add <archivos concretos>, nunca "git add .")

# 5. Verificación final antes de subir
go test ./... && go vet ./... && test -z "$(gofmt -l .)"
git diff --check

# 6. Primer push
git push -u origin master

# 7. Solo después, si todo está verde en CI:
#    crear el primer tag de release (NO ahora)
```

Si el remoto ya tiene commits, el paso 6 fallará con non-fast-forward. En ese
caso hay que decidir entre merge o rebase **antes** de forzar nada. No usar
`push --force`.

---

## Criterios de aceptación

| Criterio | Estado |
|---|---|
| Habilidades en sus teclas reales | Cumplido (44/44 canónicos) |
| Héroes transformables sin espacios ficticios | Cumplido (Silver, 4 filas) |
| Datos ambiguos presentados honestamente | Cumplido (fallback neutral + aviso) |
| TUI permite encontrar compras y situacionales | Cumplido (grupos por función) |
| Nombres y estadísticas legibles | Cumplido (ancho dinámico) |
| Mejoras visuales no alteran recomendaciones | Cumplido (solo presentación) |
| Pruebas completas verdes | Cumplido |
| LoL intacto | Cumplido |

**Lo que no puedo afirmar:** que la TUI se haya visto en una terminal real. La
verificación es automática más renders de texto a tamaños fijos.
