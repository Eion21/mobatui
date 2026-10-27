# Builds — builds de MOBAs en tu terminal

TUI (Bubble Tea + Lip Gloss) multi-juego con **datos reales**:
**League of Legends** vía [lolalytics.com](https://lolalytics.com)
(u.gg, la fuente prevista, está tras Cloudflare; Lolalytics publica los
mismos datos embebidos en su HTML SSR, que esta app decodifica) y
**Deadlock** vía [deadlock-api.com](https://deadlock-api.com). Todo
pasa por caché local con TTL y degrada a fixtures sin red. Los datos se
depuran por score de Wilson —ítems y runas con mayor win rate, orden de
habilidades— y se presentan mapeando los IDs a sus nombres reales
(español en LoL vía Data Dragon) con arte ASCII de campeones e ítems.

Al arrancar se elige el juego (1/2 o ←→); `enter` reutiliza la sesión
ya cargada (cambio instantáneo) y `r` fuerza recarga. Los roles se
cambian con `r`/`R` o clic en la píldora; las pestañas y las tarjetas
también responden al ratón. La lista **filtra al escribir**: cualquier
letra escribe (también `s` y `g`, así que "Lash" o "Grey Talon" se
buscan tecleando), `esc` cancela el filtro y un segundo `esc` vuelve
al selector; `tab` reordena (WR ▼ · Partidas · Nombre) y muestra
`X de Y`. La primera pestaña ordena por **límite inferior de Wilson**,
no por WR crudo, para no poner arriba muestras de tres partidas. Las
preferencias (último juego, héroe,
rol, pestaña, modo de retrato, orden y la pista inicial) persisten en
`os.UserConfigDir()/builds/config.json`; la cabecera muestra la
**tendencia mensual de WR** del héroe (Deadlock): con 4+ puntos se
dibuja como chispa de tendencia y con 2-3 cada punto lleva su mes
(`55,2 % (oct 25)`), en vez de bloques sin etiqueta. El **monitor de
juegos** (`m`) detecta en segundo plano el juego en curso y, en LoL, el
campeón en partida vía la Live Client Data API local (127.0.0.1:2999);
lo muestra en el pie del selector con avisos al cambiar, se apaga con
`m` y la preferencia persiste en `monitor_off`. La
lista muestra
**un campeón/héroe por fila**; el detalle es una cabecera horizontal
(nombre · roles · patch · WR · partidas · prioridad) más **pestañas**
(`tab`/`1-5`): **Ítems** (tarjetas unificadas icono+nombre+WR+pick; en LoL en
secciones CORE/SITUACIONALES/BOTAS y en Deadlock por **fase de
compra** EARLY/MID/LATE con la **ruta de compra real**: la secuencia
viene de partidas individuales (`/v1/matches/metadata` con las compras
por jugador, carga bajo demanda al abrir el héroe, streaming del JSON
y caché diaria del resultado agregado). Cada ítem es su propia
tarjeta: **compras y mejoras se intercalan en el orden de consenso**,
no en grupos separados. Las mejoras se identifican cuando ≥ 50 % de
sus compras vienen de una base. Las tarjetas tienen altura uniforme,
nombre alineado a la izquierda en dos líneas, icono de tamaño fijo y
una sola línea `~10 min · +800 almas`. Las mejoras se identifican con
`↑ Mejora de <base>`; el precio es neto para mejoras y de lista para
compras. El borde es tenue: dorado para la selección y dorado suave
para su pareja; un punto de color indica la categoría. El ancho mínimo
prioriza la lectura sobre la cantidad de columnas.
Los números conservan su posición original al plegar o filtrar tarjetas;
los ítems sin posición van al final como **OTRAS COMPRAS**, no como
pasos de la ruta. Las fases solo anotan tramos consecutivos: pueden
repetirse si los minutos no coinciden con el consenso, sin reordenarlo.
Los minutos son orientativos, no una recomendación de momento óptimo.
La fase usa el minuto propio (mediana de ruta; promedio de item-stats
como respaldo). El pie muestra solo el ítem seleccionado, paso,
minuto/coste y base cuando es mejora, en un máximo de dos líneas.
`Enter` conserva WR, pick, delta, tier, muestra y porcentajes de cadena;
`u` sigue plegando las mejoras sin cambiar la numeración.
Las cifras de WR y pick siguen saliendo de item-stats (población
completa); la ruta aporta orden, minuto, posición, patrimonio y tasas
de cadena, que solo se muestran con muestra suficiente (~100+
partidas). El popup detalla orden, posición P25–P75 y patrimonio
mediano (net_worth_at_buy, que es patrimonio acumulado, no almas en
mano) — con tier, coste en almas y borde por categoría
weapon/vitality/spirit, rankeadas por Wilson *dentro* de cada fase
para compensar el sesgo de supervivencia, con piso de uso del 8 % y
hasta 8 tarjetas por fase. En LoL el **core
son los 3 ítems más comprados** (la tabla `popularItem` de Lolalytics,
que ya excluye componentes, consumibles y wards) presentados en
**orden de compra** con su minuto medio, y el resto va a SITUACIONALES
por Wilson; los situacionales con pick rate menor al 1 % se descartan
como rarezas (con muestra absoluta grande pasaban el filtro de
partidas aunque casi nadie los compre) y de las botas se recomienda
una sola con el resto plegado en una línea informativa ("otras botas
(N)"), y el **mejor ward** (de la tabla `item`: Control Ward, amuletos
de visión) cierra la lista en su propia sección WARD, rankeado por
Wilson aparte del resto de ítems. Encima de las tarjetas hay un
**resumen jugable**: el par de **hechizos de invocador** más usado con su WR (nombres reales vía
`summoner.json` de Data Dragon, con fixture es_ES local) y el
**orden de compra medido** (`1.º X 11' → 2.º Y 19'`, de las tablas
`item1..item5`, sin repetir ítem); navegables con
←→↑↓, popup con Enter que es modal —solo esc/enter/←→ mientras está
abierto— y pie fijo con el nombre completo del ítem, la leyenda y,
cuando Lolalytics los trae, el **WR del combo real de 3 ítems**
(`builtBootSet3`),
**Resumen** (lo jugable en una pantalla: hechizos, ítems iniciales,
runa clave, orden de compra, botas, prioridad y top de ítems de
Deadlock; cada sección se dibuja solo si la fuente trae el dato),
**Runas** (LoL) y **Habilidades** (rejilla habilidad×nivel
con marcas ◆; en Deadlock las filas nombran el kit del héroe), y
**Matchups** —Deadlock (contra quién rinde peor y
mejor y con quién sinergia, vía hero-counter/synergy-stats) y LoL
(tabla `enemy` de Lolalytics, por carril: WR y partidas contra cada
enemigo frecuente)— ordenados
por límite inferior de Wilson (derrotas para difíciles, victorias
para el resto). Los
roles se conmutan con `[`/`]` y `a` alterna el estilo del retrato
(mini half-block → ASCII color con bordes y dithering → ASCII mono
con tinte dorado).

Reglas visuales: un acento por tipo de dato (dorado títulos/selección,
escala verde→rojo solo para WR con umbrales reales 58/52/48, gris para
pick y partidas), muestras pequeñas atenuadas (<500 partidas), anchos
fijos y layout responsive (por debajo de ~100 columnas se oculta el
retrato y las tarjetas pasan de 3 a 2 por fila). En LoL las tarjetas
de ítems muestran el **delta contra el WR del campeón** (`+3.6`, como
Deadlock) en vez del WR absoluto —que premia el sesgo de bola de
nieve— y marcan con `⚠ se compra yendo por delante` los ítems cuyo WR
supera en 10+ puntos al del campeón (Mejai, ítems tardíos de
Deadlock); el WR absoluto queda en la línea de detalle.

El contenido del detalle vive en un **viewport**: la cabecera y las
pestañas quedan fijas, la rueda del ratón (que salta **de fila en
fila** para no cortar tarjetas por la mitad) y PgUp/PgDn recorren lo
que no cabe, y la selección se autodesplaza para que la tarjeta activa
nunca quede fuera de pantalla. Cambiar de pestaña o de rol arranca
desde arriba: el contenido nuevo no hereda el offset del anterior.
Cuando el contenido desborda, la **línea de estado de scroll**
(`▲ más arriba · ▼ quedan N líneas`) vive en el borde inferior del
panel (ya no pegada a las pestañas) y, al bajar, el **título de la
sección activa queda fijo** (`◆ SITUACIONALES`) para que nunca se
pierda el contexto. En terminales
bajas las tarjetas de ítems pasan a **modo compacto** (sin icono, ~4
líneas) para que las secciones quepan sin recorrer la lista.

Los retratos e ítems se pre-renderizan al cargar la sesión — `View()`
solo pega strings (`internal/imgrender`: half-blocks truecolor y ASCII
con percentiles+gamma, bordes direccionales y dithering Bayer).

La primera carga de LoL descarga ~400 páginas campeón×carril con 4
workers, así que solapa latencias sin subir el ritmo contra la fuente
(un limitador global mantiene el intervalo de cortesía); en Deadlock
la carga avisa héroe por héroe (`23/26 · Abrams`) por el mismo canal.
La pantalla de carga muestra `124/400 · Ahri · middle` con su barra. Después, la
caché de disco (cuerpo gzip aparte, sin base64) sirve en ~12 s, la
caché de sesión hace que reentrar a un juego sea instantáneo, y las
entradas caducan a las 6 h. Cuando se sirven datos caducados, los
fixtures locales o se descarta algún héroe, la barra de estado lo dice
(`⚠ datos de hace 5 h`, `1 omitido`) en vez de callarlo.

```
go run .                        # TUI con datos reales (ambos juegos)
go run . --offline              # sin red: solo caché + fixtures
go run . --refresh              # invalida la caché estática
go run ./cmd/render lol-api     # lista + detalle LoL real a stdout
go run ./cmd/render deadlock-api# Deadlock real a stdout
go run ./cmd/render lol         # LoL con fixtures (offline puro)
go test ./...                   # suite completa
```

## Instalación

Con Go 1.27+ instalado, sin compilar nada a mano:

```sh
go install github.com/Eion21/builds-tui@latest
```

Binarios listos para linux (amd64/arm64), macOS (Intel/Apple Silicon) y
Windows (amd64) en [Releases](https://github.com/Eion21/builds-tui/releases),
más `.deb`/`.rpm` para Linux, tap de Homebrew
(`brew install Eion21/tap/builds`) y bucket de Scoop a partir del
primer release taggeado (`v*`), que GoReleaser publica en GitHub
Actions.

## Arquitectura

```
testdata/ugg/*.json            fixtures LoL con el formato u.gg (posición
                               1-5, pares [partidas, victorias], trampas
                               incluidas); 14 campeones, 21 builds, 5 roles
testdata/deadlock/             fixtures Deadlock: builds/ (14 héroes),
                               heroes.json, items.json, version.json e
                               iconos PNG de respaldo (cmd/genicons)
testdata/ddragon/              respaldo LoL: champion/item/runesReforged
                               (es_ES) + iconos PNG de respaldo
internal/
  model/        tipos normalizados multi-juego + reglas matemáticas de
                secuencias por juego (model.Game.SkillRules) y derivación
                de prioridad de maximización
  game/         specs por juego (nombre, roles) — añadir un juego aquí
  staticdata/   interfaz común de proveedores estáticos
  ugg/          fixtures u.gg → model (respaldo de LoL sin red)
  lolalytics/   cliente del HTML SSR de lolalytics.com (fuente real de
                LoL): parser del estado Qwik (refs base36) + tierlists
                por carril + páginas /build/ → model con reglas LoL-15;
                si el formato cambia, solo se ajusta este archivo
  deadlock/     adaptador Deadlock → model + APIClient de
                deadlock-api.com (assets + analytics, caché → HTTP →
                fixtures) + proveedor estático local para fixtures
  ddragon/      cliente Data Dragon (implementa staticdata.Provider):
                caché → HTTP → fixture
  clean/        pipeline: filtro de muestra mínima + ranking por límite
                inferior de Wilson (95%) + core de LoL por pick rate en
                orden de compra + extracción de top ítems/botas/runas/
                secuencia
  cache/        caché en disco con TTL y stale-if-error
  imgrender/    imágenes → arte pre-renderizado: half-blocks truecolor
                (ítems y mini de cabecera) y ASCII de calidad para
                retratos (percentiles+gamma, bordes direccionales,
                dithering Bayer, color por celda o tinte dorado)
  ascii/        PNG → ASCII por rampa de luminancia (mono/color, modo
                denso, half-block); usado por genicons/tests
  ui/           TUI: selector de juego, lista (1 por campeón/héroe) y
                detalle con pestañas de rol, arte ASCII tintado, barras
                de win rate y franja de niveles alineada
data/                          runtime (caché); se autogenera
```

## Añadir un juego nuevo

1. `model.Game` + reglas en `SkillRules()` (alfabeto, longitud, puntos
   máximos, niveles bloqueados).
2. `game.All`: nombre y roles.
3. Adaptador (`internal/<fuente>`) que produzca `model.ChampionBuilds`
   y un `staticdata.Provider` (nombres e iconos).
4. Fixtures en `testdata/<juego>/` e iconos vía `cmd/genicons`.
5. Cablear en `main.go` (Loader.Providers) — la TUI lo hereda todo.

## Por qué Wilson y no win rate crudo

Un ítem al 100% con 3 partidas no es mejor que uno al 53% con 40.000.
El pipeline exige muestra mínima (≥100 partidas, o ≥20 con ≥5% de pick
rate) y ordena por el límite inferior del intervalo de confianza de
Wilson: los fixtures incluyen trampas deliberadas (100% con 3 partidas,
58% con 120) que los tests verifican que no encabezan el ranking.

En LoL, además, el WR premia lo que se compra en casi todas las
partidas: componentes (Sello de acero), consumibles y wards encabezaban
el core. Por eso el core sale de la tabla `popularItem` de Lolalytics
—ya filtrada a ítems completados— y se ordena por pick rate y después
por minuto de compra; Wilson sigue ordenando los situacionales y las
runas. Un test de regresión recorre las páginas cacheadas y falla si un
ítem del core es componente, consumible, ward o barato.

## Sobre las fuentes reales

- **u.gg está descartada**: stats.u.gg responde 403 con desafío JS de
  Cloudflare a cualquier cliente HTTP. Lolalytics sirve datos
  equivalentes sin bloquear; el precio es que la primera carga de LoL
  descarga ~200 páginas (~1-2 min, luego caché 6 h) y el scraping es
  cortés (pausa de 150 ms entre peticiones, UA identificado).
- Los iconos de ítems de Deadlock usan `shop_image` (cuadrados 200×200
  de la tienda); el campo `image` es el icono interno del mod, con
  huecos 404.
- Los WR altos de ítems de snowball/tardíos (Robaalmas de Mejai, ítems
  de Deadlock comprados al final) son sesgo de supervivencia: se
  compran cuando ya vas ganando. El ranking es correcto con lo que
  reporta la fuente.

## Notas sobre los datos de respaldo

Los fixtures de Data Dragon usan nombres en español con los IDs reales
conocidos; con red, la app descarga los datos oficiales vía caché y
sustituye cualquier aproximación. Los iconos de respaldo son
deterministas y abstractos (`cmd/genicons`); con red se usan los
iconos oficiales.
