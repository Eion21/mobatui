# MOBA TUI

```text
 __  __  ___  ____    _    _____ _   _ ___
|  \/  |/ _ \| __ )  / \  |_   _| | | |_ _|
| |\/| | | | |  _ \ / _ \   | | | | | || |
| |  | | |_| | |_) / ___ \  | | | |_| || |
|_|  |_|\___/|____/_/   \_\ |_|  \___/|___|
```

**Builds, runas y estadísticas de League of Legends y Deadlock — directo en tu terminal.**

[English README](README.en.md)

Una interfaz de terminal (TUI) rápida para **League of Legends** y **Deadlock** que convierte datos reales de partidas en builds, órdenes de habilidades y estadísticas fáciles de leer de un vistazo mientras juegas.

Desarrollada en Go con [Bubble Tea](https://github.com/charmbracelet/bubbletea) y [Lip Gloss](https://github.com/charmbracelet/lipgloss).

---

## Capturas de pantalla

### Vista de build e ítems (Deadlock)
> Tarjetas en truecolor por fase (`EARLY`, `MID`, `LATE`), evolución de componentes (`↑`), minuto estimado, costo en almas, orden de habilidades (`max 3 > 1 > 2 > 4`) y tendencia de win rate por parche.

![Vista de build e ítems en Deadlock](docs/screenshots/deadlock-build.jpg)

### Selector de juego y lista de héroes

| Menú principal y monitor activo | Lista filtrable ordenada por Wilson Score |
| --- | --- |
| ![Menú principal y monitor activo](docs/screenshots/game-select.png) | ![Lista filtrable ordenada por Wilson Score](docs/screenshots/hero-list.jpg) |

---

## Características principales

- **Datos reales, sin humo:** Builds generadas a partir de secuencias de compra reales y evaluación estadística conservadora.
- **Confianza visible:** Etiquetas *alta / moderada / baja* según tamaño de muestra e incertidumbre, nunca presentadas como «probabilidad de ganar».
- **Arte en terminal (Truecolor):** Iconos de ítems y retratos de héroes/campeones renderizados directamente en consola.
- **Funciona offline:** Caché local en disco con TTL, degradación controlada (stale-if-error) y fallback a fixtures incluidos.
- **Monitor de partida (`m`):** Detecta automáticamente si el juego está abierto y, en League of Legends, identifica tu campeón en tiempo real mediante la *Live Client Data API*.
- **Identificable y actualizable:** `mobatui --version` muestra versión, commit, Go y plataforma. Al arrancar avisa si hay una versión nueva en GitHub (sin bloquear y sin autoactualizarse); se desactiva con `-no-update-check` o `-offline`.

### Deadlock (vía [deadlock-api.com](https://deadlock-api.com))
- **Generador híbrido de builds:** Combina estadística por ítem con un motor de progresión que respeta cadenas de mejora, consumo de componentes y límites de inventario.
- **Fases y contexto:** Rutas divididas en **EARLY / MID / LATE**, extensiones para partidas largas e ítems **situacionales** agrupados por función (anti-heal, defensa física/espiritual, resistencia a CC, supervivencia y daño extra).
- **Habilidades reales:** Orden de subida mapeado a las teclas reales del héroe (`signature1..4`), consolidando formas transformadas en sus 4 ranuras reales.
- **Meta y enfrentamientos:** Tendencia de win rate entre parches, matchups y sinergias.

### League of Legends (vía [lolalytics.com](https://lolalytics.com))
- **Todo en un lugar:** Builds de objetos, páginas de runas, orden de habilidades y hechizos de invocador.
- **Ranking por cota inferior de Wilson:** Evita que muestras pequeñas con win rates inflados superen a builds probadas en decenas de miles de partidas.
- **Ruta de compra e inventario final (`Ruta`):** Orden de compras con minuto medio, botas, inventario con ranura explícita y motivo cuando falta un objeto.
- **Alternativas por función (`Alt.`):** Hasta 12 candidatos distintos agrupados en categorías, repartidos por rondas para que ninguna quede desatendida.

---

## Requisitos e instalación

- **Go 1.27** o superior para compilar desde el código fuente.
- Una **terminal con soporte Truecolor** (Kitty, Alacritty, WezTerm, Ghostty, Windows Terminal, iTerm2, etc.) para visualizar correctamente los iconos y retratos.
- **Plataformas:** Linux es la plataforma principal; macOS y Windows están soportados en código y CI.

```bash
go install github.com/Eion21/mobatui@latest
mobatui
```

O compilando desde el repositorio:

```bash
git clone https://github.com/Eion21/mobatui
cd mobatui
go build -o mobatui .
./mobatui
```

La caché se guarda en el directorio de caché del usuario (por defecto `~/.cache/mobatui` en Linux) y la configuración en `~/.config/mobatui/config.json`. No se escriben archivos dentro del repositorio.

### Flags disponibles

| Flag | Por defecto | Descripción |
| --- | --- | --- |
| `-version` / `--version` | — | Muestra versión, commit, Go y plataforma, y sale sin tocar caché ni red |
| `-data` | `~/.cache/mobatui` | Directorio para la caché en tiempo de ejecución |
| `-offline` | `false` | Modo sin red: usa únicamente caché local y fixtures (también desactiva el aviso de versión) |
| `-no-update-check` | `false` | No comprobar si hay una versión nueva en GitHub |
| `-refresh` | `false` | Invalida la caché estática antes de iniciar |
| `-locale` | `es_ES` | Idioma de Data Dragon para League of Legends |
| `-ugg` | `testdata/ugg` | Directorio de datos/dumps de u.gg |
| `-deadlock` | `testdata/deadlock/builds` | Directorio de fixtures/builds de Deadlock |

---

## Controles

| Tecla | Acción |
| --- | --- |
| `1` / `2`, `←` `→` | Seleccionar juego (`1` League of Legends, `2` Deadlock) |
| `g` | Cambiar de juego rápidamente |
| `enter` | Abrir héroe/campeón o abrir detalle del ítem seleccionado |
| `escribir` | Filtrar la lista de héroes/campeones en tiempo real |
| `tab` | Cambiar criterio de orden en la lista / cambiar pestaña en detalle |
| `1`-`7` | Saltar directamente a una pestaña (LoL: `Ítems`, `Ruta`, `Alt.`, `Resumen`, `Runas`, `Habil.`, `Match.`) |
| `←↑↓→`, `hjkl` | Mover selección entre tarjetas o filas |
| `r` / `R` | Forzar recarga (o rotar rol adelante/atrás en vista de detalle) |
| `u` | Plegar o desplegar tarjetas de mejora (Deadlock) |
| `M` | Conmutar el inventario de BOTTOM entre antes y después de la misión |
| `v` / `V` | Abrir la release nueva en el navegador / descartar el aviso |
| `a` | Cambiar modo de retrato |
| `m` | Activar o desactivar el monitor de juego |
| `?` | Mostrar/ocultar ayuda de atajos |
| `esc` | Cerrar popup, limpiar filtro o volver atrás |
| `rueda del ratón` | Desplazar vista |

---

## ¿Cómo se calculan las builds?

El generador es deliberadamente conservador respecto a lo que afirma:

```text
┌──────────────┐      ┌──────────────┐      ┌──────────────────┐
│  APIs / HTML │ ───▶ │ Caché local  │ ───▶ │  Bubble Tea TUI  │
│  (red/fix)   │      │ TTL + stale  │      │ vistas + tarjetas│
└──────────────┘      └──────────────┘      └──────────────────┘
        │                     ▲                        │
        └──── fallback ───────┘                        ▼
                                   Selección → builds, skills, stats
```

1. **Ruta observada:** Secuencias de compra extraídas de partidas reales, ordenadas por precedencia de consenso.
2. **Evaluación estadística:** Cada ítem candidato se evalúa según popularidad, frecuencia ajustada por fase, rendimiento ajustado, tamaño muestral, incertidumbre estadística y consistencia (mediante distribución posterior Beta-Binomial y cota inferior de Wilson).
3. **Motor de progresión:** Valida que la secuencia sea mecánicamente comprable en partida (existencia de cadenas de mejora en el catálogo, consumo real de componentes, límite de ranuras de inventario y ausencia de ciclos).
4. **Build híbrida:** Combina el respaldo estadístico con la validez mecánica, registrando el motivo de inclusión o exclusión de cada objeto.

> **Nota sobre el Win Rate:** Un porcentaje de victoria alto **no** se interpreta como causalidad. Los ítems caros comprados cuando un equipo ya va ganando tienen win rates inflados; el sistema corrige este sesgo en lugar de premiarlo.

### Ruta sugerida y alternativas (League of Legends)

La pestaña **`Ruta`** muestra el orden de compra y el inventario final; la pestaña **`Alt.`** agrupa candidatos por función.

> **Importante — ruta compuesta, no secuencia observada.** Lolalytics publica `item1..item5` como *posiciones de compra independientes*: "qué se compra primero", "qué se compra segundo", etc. Cada paso es el objeto más frecuente en esa posición, pero **la ruta resultante no es la secuencia de ninguna partida real**. Por eso se rotula *"basada en compras observadas"*.

Las combinaciones de tres objetos (`builtBootSet3`) se mantienen **fuera** del planificador: publican el mismo conjunto en varias permutaciones y su semántica no está demostrada, así que no se usan para inferir orden.

**Reglas de inventario y misión (temporada 2026):**

| Rol | Ranura de misión | Efecto |
| --- | --- | --- |
| BOTTOM | Botas | Antes de completarla las botas ocupan un espacio principal; después pasan a la ranura y quedan 6 espacios principales |
| SUPPORT | Guardianes de control | El objeto de soporte y las botas ocupan espacios principales; **no** hereda la ranura de botas de BOTTOM |
| MID | — | La misión mejora las botas Tier 2 a Tier 3, pero no libera espacio |
| TOP / JUNGLE | — | Inventario normal de 6 espacios |

La tecla `M` conmuta la vista de BOTTOM entre antes y después de la misión. Es una **preferencia de vista**: la app no deduce el estado real de la partida. Cuando falta evidencia para el sexto objeto, el plan se declara **incompleto** con el motivo, en lugar de inventarlo.

### Fuentes y fallback

- **Lolalytics** es la fuente principal: aporta posiciones de compra, botas, combinaciones y hechizos.
- **u.gg** actúa de respaldo cuando Lolalytics no está disponible. **No publica tablas por posición**, así que con u.gg no se reconstruye la ruta: se muestran las estadísticas disponibles y se indica el motivo.
- Un catálogo incompleto o un parche que no cuadra se informan por separado: *"parche incompatible"* (ningún id del catálogo) frente a *"N objetos no están en el catálogo"* (ids sueltos ausentes).
- Los objetos de misión de soporte se identifican por id y se marcan como tales: **no se les inventa un minuto de compra**.

### Niveles de confianza

- **Alta:** Muestra grande, intervalo de incertidumbre estrecho y consistencia en los datos.
- **Moderada:** Datos utilizables, pero sin volumen suficiente para afirmaciones categóricas.
- **Baja:** Se muestra por completitud; debe tomarse solo como referencia.

---

## Limitaciones conocidas

- **Consenso estadístico, no juego óptimo:** Las builds reflejan qué funciona en el agregado de partidas, no una garantía de victoria en una partida concreta.
- **Cobertura de héroes en Deadlock:** El catálogo incluye 65 héroes; 44 cuentan con suficientes partidas en caché para generar rutas completas (el resto muestra estadísticas generales sin orden de compra, y 2 héroes en desarrollo aún no tienen kit publicado).
- **Etiquetas situacionales:** Se derivan de propiedades verificables del objeto (nombre y categoría), no de un análisis causal contra héroes específicos.
- **Dependencia de marcado en LoL:** Los datos de League of Legends se extraen del HTML renderizado de Lolalytics; cambios en su estructura web pueden requerir actualizar el parser.
- **Ruta de LoL compuesta:** La ruta se construye con las posiciones de compra más frecuentes por separado; no representa una partida real. Con u.gg (respaldo) no hay posiciones y no se reconstruye ruta.
- **Estado de misión manual en BOTTOM:** La app no puede conocer si completaste la misión; `M` es una preferencia de vista para alternar antes/después.
- **Aviso de versión, no autoactualización:** La comprobación solo informa de que existe una release nueva; nunca descarga ni reemplaza el binario.
- **Rutas de Deadlock y HTTP 504:** la API de muestras puede devolver error temporal. En ese caso se conservan las estadísticas generales y la ruta se reintenta al reabrir el héroe.
- **Secuencia de habilidades de Kai'Sa:** Lolalytics publica una secuencia con seis puntos en Q y seis en E, imposible con el máximo de cinco. Se rechaza esa secuencia y se conserva la prioridad Q > E > W, que la fuente sirve aparte.

---

## Desarrollo

```bash
go test ./...                    # Suite completa (sin conexión a internet)
go vet ./...                     # Análisis estático
gofmt -l .                       # Verificación de formato (debe salir vacío)
go run ./cmd/render deadlock     # Renderizado en texto plano para depurar layouts
```

### Estructura del proyecto

```text
.
├── cmd/
│   ├── mobatui/        # (o raíz con main.go) Punto de entrada principal
│   └── render/         # Renderizador de texto/fixtures para pruebas de layout
├── docs/
│   └── screenshots/    # Capturas usadas en el README
├── internal/
│   ├── deadlock/       # Cliente API, rutas, evaluación y generador híbrido
│   ├── progression/    # Catálogo, cadenas de mejora y simulación de inventario
│   ├── lolalytics/     # Cliente de estadísticas de LoL
│   ├── ddragon/        # Datos estáticos e imágenes de Data Dragon
│   ├── clean/          # Puntuación de Wilson y limpieza de builds
│   ├── ui/             # Vistas Bubble Tea, tarjetas y navegación
│   └── cache/          # Caché en disco (TTL, stale-if-error)
└── testdata/           # Fixtures para tests y modo offline
```

---

## Fuentes de datos y créditos

- **Deadlock:** [deadlock-api.com](https://deadlock-api.com) (API comunitaria). Los datos e imágenes pertenecen a Valve.
- **League of Legends:** [lolalytics.com](https://lolalytics.com) para estadísticas y [Data Dragon](https://developer.riotgames.com/docs/lol#data-dragon) para nombres e iconos.
- **Live Client Data API** (`127.0.0.1:2999`) para detectar el campeón activo en partida.

*Deadlock es una marca registrada de Valve Corporation. League of Legends y Riot Games son marcas registradas de Riot Games, Inc. Este proyecto no está afiliado, respaldado ni patrocinado por ninguna de las dos compañías.*

## Licencia

MIT — consulta el archivo [LICENSE](LICENSE). Los datos y recursos visuales de los juegos siguen siendo propiedad de sus respectivos dueños.
