# MOBA TUI v0.2.0 — Notas de release (borrador)

**Estado:** borrador preparado tras la auditoría final. Sin tag, push ni publicación de release.

## Resumen

v0.2.0 añade tres cosas: la app ya **se identifica** (`--version`), **avisa** cuando hay una versión nueva sin bloquear el arranque, y las builds de League of Legends pasan a ser **completas**: ruta de compra con inventario final y alternativas agrupadas por función.

Base: `v0.1.1` (`05e64b4`). Rama: `feature/v0.2.0`.

## Cambios incluidos desde `v0.1.1`

### Identificación de versión (`34a512d`)

- `mobatui --version` y `-version` muestran versión, fuente, commit, Go y plataforma.
- La versión se resuelve con precedencia **ldflags > metadatos VCS > `dev`**. GoReleaser inyecta el tag, así que un binario de release anuncia la versión real aunque el árbol esté sucio.
- `--version` no crea caché, no carga datos y no arranca la TUI (verificado: no toca la red ni el disco).
- `--help` muestra la versión y la lista de opciones.

### Aviso de versión nueva (`f7715a2`)

- Consulta las releases de GitHub **en segundo plano**, con timeout propio y caché de 24 h.
- **No retrasa el arranque:** con el proxy caído el arranque medido es de ~6 ms.
- El aviso vive en el footer. `v` abre la release en el navegador, `V` lo descarta.
- No se usó `u` porque ya pliega las mejoras de Deadlock.
- Se desactiva con `-no-update-check` o con `-offline`.
- **Solo avisa: no descarga ni autoactualiza.**

### Catálogo de objetos y planificador (`2bc791d`)

- `ddragon.ItemCatalog()` expone objetos con etiquetas, coste, cadenas de combinación y mapas, reutilizando la **misma URL y caché** que `Items()`: no añade peticiones.
- `internal/lolitems`: 13 categorías, separando lo **declarado** por Data Dragon de lo **derivado** por reglas propias; filtrado de elegibilidad por mapas, consumibles, trinkets y recetas. La anticuración es una lista de ids verificada contra descripciones reales.
- `internal/lolbuild`: construye pasos por posición de compra, botas, core intacto, alternativas por función e inventario final.
- Las combinaciones de tres objetos (`builtBootSet3`) quedan **fuera** del planificador: su semántica no está demostrada.

### Interfaz de builds (`40f79c3`)

- Nueva pestaña **`Ruta`**: compras ordenadas con minuto medio, botas, inventario final con ranura explícita y motivo cuando falta un objeto.
- Nueva pestaña **`Alt.`**: hasta 12 candidatos distintos agrupados por función; un objeto multicategoría se muestra completo solo en su primera categoría.
- **`M`** conmuta el inventario de BOTTOM entre antes y después de misión (solo BOT: es el único rol donde la misión cambia espacios).
- Los planes se cachean por sesión con clave que incluye campeón, rol, parche, misión, fuente, huella de estadísticas y huella de catálogo: el render no recalcula y una recarga con el mismo total de partidas **sí** invalida.
- Ambas vistas envuelven texto al ancho útil: no desbordan ni cortan palabras a 40, 60, 80 ni 118 columnas.

## Reglas de inventario (temporada 2026)

| Rol | Ranura de misión | Efecto |
| --- | --- | --- |
| BOTTOM | Botas | Antes: botas en espacio principal. Después: botas a la ranura, 6 espacios principales |
| SUPPORT | Guardianes de control | Objeto de soporte y botas en espacios principales; **no** hereda la ranura de BOTTOM |
| MID | — | Mejora las botas Tier 2 a Tier 3, sin liberar espacio |
| TOP / JUNGLE | — | Inventario normal de 6 espacios |

Cuando falta evidencia para el sexto objeto, el plan se declara **incompleto** con el motivo en vez de inventarlo.

## Sobre la ruta de League of Legends

`item1..item5` son **posiciones de compra independientes**: "qué se compra primero", "qué se compra segundo", etc. La ruta que muestra la app es una **construcción estadística** con la opción más frecuente de cada posición, **no la secuencia de una partida real**. Por eso se rotula *"basada en compras observadas"*.

Con **u.gg** como respaldo no hay tablas por posición, así que no se reconstruye ruta: se muestran las estadísticas disponibles y se explica el motivo.

## Verificaciones ejecutadas

- `go build ./...` — sin errores.
- `go test ./... -count=1` — 17 paquetes OK, 0 fallos, sin red ni caché personal.
- `go vet ./...` — sin hallazgos.
- `go test -race ./...` — sin carreras.
- `go run ./cmd/render lol` y `go run ./cmd/render deadlock` — ambos OK.
- `gofmt -l .` — sin diferencias. `git diff --check` — limpio.
- Cobertura de los paquetes nuevos: `updatecheck` 94.9 %, `lolbuild` 94.0 %, `version` 93.3 %, `lolitems` 82.0 %.
- Binario compilado con los flags de GoReleaser (`-s -w -X ...version.injected=0.2.0`) y probado desde un directorio ajeno al repositorio.
- Deadlock sin cambios funcionales: `internal/deadlock`, `internal/clean`, `internal/progression` y `internal/ugg` no se modificaron.

## Riesgos y limitaciones conocidas

- **Sin revisión visual interactiva:** la auditoría no dispuso de TTY. Se verificó el render en texto y el ancho a 40/60/80/118 columnas, pero **no** la disposición percibida en pantalla. Pendiente de comprobación manual.
- **Ruta compuesta, no secuencia real** (ver arriba).
- **Estado de misión manual en BOTTOM:** la app no puede saber si completaste la misión; `M` es una preferencia de vista.
- **Vistas `Ruta` y `Alt.` son textuales**, sin iconos de ítem ni popup de detalle. Deliberado: son vistas de lectura, y las tarjetas a 40 columnas serían inutilizables.
- **Dependencia de marcado en LoL:** los datos se extraen del HTML de Lolalytics; cambios en su estructura pueden requerir actualizar el parser.
- **Aviso de versión, no autoactualización.**

## Checklist previo a publicar

- [ ] Revisión visual manual en Linux (ver procedimiento en el informe de la fase 7)
- [ ] Confirmar tag objetivo `v0.2.0`
- [ ] Verificar que la CI pasa en los tres sistemas
- [ ] No publicar sin aprobación explícita
