# Distribución por gestores de paquetes

Estado: **preparado y verificado en local, NO publicado.**

Este documento describe cómo se instalaría MOBA TUI mediante WinGet,
Homebrew y Scoop, qué falta para publicarlo y qué coste de mantenimiento
tiene. Nada de esto está activo todavía: los repositorios externos no
existen y los manifiestos no se han enviado.

---

## 1. Estado real de cada gestor

| Gestor | Plataforma | Estado | Qué falta |
|---|---|---|---|
| **Homebrew** | macOS | Manifiesto generado y validado | Crear `Eion21/homebrew-tap`, primera publicación |
| **Scoop** | Windows | Manifiesto generado y validado | Crear `Eion21/scoop-bucket`, primera publicación |
| **WinGet** | Windows | Manifiestos generados y validados contra el esquema oficial | Fork de `winget-pkgs`, PR a Microsoft, revisión humana |
| **Chocolatey** | Windows | **No evaluado** | Segunda etapa, como se pidió |

**Ninguno está disponible para el usuario final.** Escribir
`winget install Eion21.MOBATUI` hoy falla: el paquete no existe en el
catálogo de Microsoft.

---

## 2. Cambio de empaquetado necesario

### El problema

La release publicada de v0.2.1 incluye **`mobatui_0.2.1_windows_amd64.tar.gz`**.
Ni WinGet ni Scoop aceptan `.tar.gz`:

- **WinGet** declara un instalador portable y espera un archivo que pueda
  extraer; su manifiesto usa `InstallerType: zip`.
- **Scoop** extrae el archivo con `Expand-Archive`, que **solo** entiende
  `.zip`.

### La solución

`format_overrides` en el archivo de Windows, manteniendo `.tar.gz` en el
resto:

```yaml
archives:
  - id: mobatui
    ids:
      - mobatui
    name_template: >-
      {{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}
    format_overrides:
      - goos: windows
        formats: [zip]
```

Efecto comprobado en un snapshot local:

- `mobatui_<ver>_windows_amd64.zip` ← **nuevo**, contiene `mobatui.exe`
- `mobatui_<ver>_linux_amd64.tar.gz` ← sin cambios
- `mobatui_<ver>_darwin_arm64.tar.gz` ← sin cambios
- `mobatui-render_*` ← sin cambios (no lo consume ningún gestor)

**Las URLs ya publicadas siguen siendo válidas**: solo se añade un
archivo, no se cambia ninguno existente.

**El verificador impide el error de publicar para v0.2.1.** Consulta la
API de GitHub y rechaza cualquier manifiesto cuyo asset no exista en la
release indicada. Probado con el ZIP de un snapshot local: el archivo
existía en `dist/` y tenía hash válido, pero **no estaba publicado**, y
el verificador lo rechazó. Los manifiestos de v0.2.1 **no se pueden
publicar** aunque alguien ejecute el workflow: falla antes de escribir.

### Consecuencia

Este cambio **solo tendrá efecto en la próxima release** (v0.2.2 o
posterior). Los gestores no pueden publicarse para v0.2.1 sin volver a
empaquetar esos binarios, cosa que este trabajo no hace.

---

## 3. Los tres manifiestos

GoReleaser v2.19.0 los genera de forma nativa. No hace falta ningún
script propio ni proceso externo.

### WinGet

```yaml
winget:
  - name: mobatui
    ids: [mobatui]
    package_identifier: Eion21.MOBATUI
    package_name: MOBA TUI
    publisher: Eion21
    license: MIT
    # ...
    repository:
      owner: Eion21
      name: winget-pkgs
      branch: master
      token: "{{ .Env.WINGET_TOKEN }}"
      pull_request:
        enabled: true
        base: { owner: microsoft, name: winget-pkgs, branch: master }
        draft: true
```

Genera **tres** archivos, que es lo que Microsoft exige:

```
manifests/e/Eion21/MOBATUI/<version>/Eion21.MOBATUI.yaml            # versión
manifests/e/Eion21/MOBATUI/<version>/Eion21.MOBATUI.installer.yaml  # instalador
manifests/e/Eion21/MOBATUI/<version>/Eion21.MOBATUI.locale.en-US.yaml
```

Verificado contra el esquema oficial `1.12.0` de Microsoft:
**los tres validan**.

El instalador generado es portable con alias de comando, que es lo que
permite escribir `mobatui` tras instalar:

```yaml
Installers:
  - Architecture: x64
    NestedInstallerType: portable
    NestedInstallerFiles:
      - RelativeFilePath: mobatui.exe
        PortableCommandAlias: mobatui
    InstallerUrl: .../mobatui_<ver>_windows_amd64.zip
    InstallerSha256: <64 hex>
    UpgradeBehavior: uninstallPrevious
```

`UpgradeBehavior: uninstallPrevious` es lo que hace que
`winget upgrade` funcione sin dejar dos copias.

**Colisión de identificador:** comprobado con la API de GitHub que
`manifests/e/Eion21` **no existe** en `microsoft/winget-pkgs` (HTTP 404),
igual que `Eion21.MOBATUI`, `Eion21.MobaTUI` y `Eion21.Mobatui`. El
identificador está libre.

### Homebrew

**DECISIÓN REVISADA.** La primera versión de este diseño incluía un hook
que eliminaba `com.apple.quarantine` automáticamente al instalar. **Se ha
retirado.** Quitar el atributo de cuarentena equivale a silenciar el
control de Gatekeeper: el usuario instala un binario de origen remoto sin
ver la advertencia del sistema. Aunque es una práctica extendida, no debe
ser la solución por defecto ni presentarse como definitiva.

Configuración actual: **Cask sin hooks**. El primer arranque mostrará el
diálogo de Gatekeeper y el usuario decide.

```yaml
homebrew_casks:
  - name: mobatui
    ids: [mobatui]
    repository: { owner: Eion21, name: homebrew-tap, branch: main }
    directory: Casks
    # sin hooks: la cuarentena se respeta
```

El Cask generado cubre **macOS arm64 e Intel** con hashes distintos,
más Linux como extra.

#### Las tres opciones, comparadas

**A. Cask con binario sin firmar y sin tocar la cuarentena** ← *elegida*

| | |
|---|---|
| Ventajas | Instalación inmediata (sin toolchain de Go). Hashes fijados por release. No silencia nada. Cambiar a firma después no rompe a nadie |
| Limitaciones | **Primer arranque bloqueado por Gatekeeper.** El usuario debe ir a *Ajustes del Sistema → Privacidad y seguridad → «Abrir igualmente»*. Instalación con fricción |
| Pruebas necesarias | En macOS real: `brew install --cask`, verificar el diálogo, comprobar que «Abrir igualmente» funciona, y que `mobatui --version` responde tras aprobarlo |

**B. Fórmula compilada desde código fuente**

| | |
|---|---|
| Ventajas | **Sin diálogo de Gatekeeper**: Homebrew compila localmente, y un binario compilado en la máquina del usuario no lleva cuarentena ni firma de terceros. Es la vía limpia sin pagar a Apple. Reproducible y auditable |
| Limitaciones | Requiere **Go 1.27+** instalado (Homebrew lo pone como dependencia, ~250 MB). Compilación de 30–90 s. Necesita fórmula mantenida a mano (GoReleaser genera Casks, no fórmulas de este tipo). Descarga el código, no el binario: no valida el artefacto publicado |
| Pruebas necesarias | En macOS real (Intel y Apple Silicon): `brew install eion21/tap/mobatui`, medir tiempo, verificar `mobatui --version`, y que `go` como dependencia no rompa a quien ya lo tiene |

**C. Firmar y notarizar el binario** (solución definitiva)

| | |
|---|---|
| Ventajas | **La única sin fricción y sin silenciar nada.** macOS acepta el binario porque Apple lo ha notarizado. Cask con instalación normal |
| Limitaciones | **Cuenta de desarrollador de Apple: 99 USD/año.** Requiere certificado *Developer ID Application*, `codesign` + `notarytool` en el workflow, guardar el certificado como secreto, y ~10 min por release de espera de notarización |
| Pruebas necesarias | Verificar con `codesign -dv --verbose=4` y `spctl -a -vv`, y que `brew install` no muestre diálogo en una máquina limpia |

#### Recomendación

**Empezar por A, con la limitación documentada en el README**, y evaluar
**B** si la fricción resulta inaceptable. **C** es el destino correcto,
pero su coste anual debe decidirse con conocimiento de causa, no como
efecto colateral de una decisión técnica.

Lo que **no** se hace:

- ❌ No se elimina `com.apple.quarantine` automáticamente.
- ❌ No se pide al usuario que ejecute `spctl --master-disable`.
- ❌ No se instruye a desactivar Gatekeeper en el README.

Si en el futuro se añade la limpieza de cuarentena al Cask, debe ser una
decisión explícita, documentada y visible para el usuario —no un `hook`
silencioso—.

### Scoop

```yaml
scoops:
  - name: mobatui
    ids: [mobatui]
    repository: { owner: Eion21, name: scoop-bucket, branch: main }
    directory: bucket
```

Manifiesto generado:

```json
{
  "version": "0.2.1",
  "architecture": {
    "64bit": {
      "url": ".../mobatui_0.2.1_windows_amd64.zip",
      "bin": ["mobatui.exe"],
      "hash": "<64 hex>"
    }
  },
  "homepage": "https://github.com/Eion21/mobatui",
  "license": "MIT",
  "description": "..."
}
```

---

## 4. Repositorios externos necesarios

Ninguno existe todavía (comprobado: HTTP 404).

| Repositorio | Contenido | Visibilidad |
|---|---|---|
| `Eion21/homebrew-tap` | `Casks/mobatui.rb` | Público |
| `Eion21/scoop-bucket` | `bucket/mobatui.json` | Público |
| `Eion21/winget-pkgs` (fork) | Solo para abrir el PR | Público |

Comandos objetivo una vez publicados:

```bash
brew install --cask eion21/tap/mobatui
```
```powershell
scoop bucket add eion21 https://github.com/Eion21/scoop-bucket
scoop install eion21/mobatui
```
```powershell
winget install Eion21.MOBATUI
```

---

## 5. Separación publicación / manifiestos

**Decisión: dos workflows.**

| Workflow | Cuándo | Qué hace |
|---|---|---|
| `release.yml` | push de tag `v*` | Compila, publica binarios y GitHub Release. **Salta** los tres gestores. |
| `publish-packages.yml` | **manual** | Regenera manifiestos y los escribe en sus repos. |

### Por qué `release.yml` los salta

```yaml
args: release --clean --skip=winget,scoop,homebrew
```

Esto no es cosmético. Sin esos `--skip`, GoReleaser **aborta la release
completa** si falta un token:

```
error=template: failed to apply "{{ .Env.SKIP_PACKAGE_UPLOAD }}":
      map has no entry for key "SKIP_PACKAGE_UPLOAD"
```

Lo verifiqué: un fallo de configuración en un gestor de paquetes podía
impedir que se publicaran los binarios. Con `--skip` esto es imposible.

### Por qué NO se usa `skip_upload` con plantilla

La primera versión usaba `skip_upload: '{{ .Env.SKIP_PACKAGE_UPLOAD }}'`.
Falla de dos maneras:

1. Sin la variable definida, **aborta la release**.
2. `default` de sprig **no está disponible** en las plantillas de
   GoReleaser (`function "default" not defined`).

Por eso el control se hace con `--skip` en la línea de comandos, que es
explícito y no depende de variables.

### Tokens y permisos mínimos

| Secreto | Permiso necesario | Alcance |
|---|---|---|
| `GITHUB_TOKEN` (implícito) | `contents: write` **solo en `release.yml`** | Eion21/mobatui |
| `HOMEBREW_TAP_TOKEN` | `contents: write` | Solo `Eion21/homebrew-tap` |
| `SCOOP_BUCKET_TOKEN` | `contents: write` | Solo `Eion21/scoop-bucket` |
| `WINGET_TOKEN` | `public_repo` (fork + PR) | Solo `Eion21/winget-pkgs` |

`publish-packages.yml` declara `permissions: contents: read` sobre el
repositorio principal: **no puede** modificar MOBA TUI ni sus releases.

### La secuencia real (corregida)

La versión anterior de este diseño **no publicaba nada**: usaba
`--skip=publish`, y los pipes de cask, scoop y winget viven **dentro** de
la fase de publicación (`internal/pipe/publish/publish.go`). Omitirla
producía un no-op silencioso que parecía correcto en los logs.

| Fase | Comando | Escribe en |
|---|---|---|
| **1. Generar** | `goreleaser release --clean --skip=publish,validate` | **Solo `dist/`** (`skip_upload: true`) |
| **2. Verificar** | `go run -tags packages ./cmd/pkgcheck …` | Nada (solo lectura) |
| **3. Publicar** | `goreleaser release -f .goreleaser-publish.yaml --fail-fast` | Tap, bucket, PR de WinGet |
| **4. Comprobar** | `diff` de assets y `checksums.txt` | Nada |

**Garantía central:** si la fase 2 falla, la fase 3 no se ejecuta. El
paso de publicación es un paso posterior del mismo job: GitHub Actions
detiene el job en el primer paso que falla.

**`release.disable: true`** en `.goreleaser-publish.yaml` evita tocar la
GitHub Release sin necesidad de `--skip=publish`, que arrastraría la
omisión de cask/scoop/winget.

**`--fail-fast` es obligatorio.** Los tres pipes declaran
`ContinueOnError() = true`, así que sin él un fallo en Homebrew **no
impide** que Scoop escriba: quedan escrituras parciales. Verificado con
un destino roto: sin `--fail-fast`, Scoop publicaba; con `--fail-fast`,
no.

### Garantías verificadas de `publish-packages.yml`

Cada afirmación se comprobó ejecutando los pasos y comparando el estado
de la release antes y después.

| Garantía | Cómo se cumple | Prueba |
|---|---|---|
| **No modifica la GitHub Release** | `permissions: contents: read`; no hay `contents: write` en ningún paso | Inventario de assets antes/después: **idéntico** |
| **No reemplaza binarios ni checksums** | `release.disable: true` en la config + `Assert release unchanged` compara assets y `checksums.txt` | `checksums.txt` antes/después: **idéntico** |
| **Se publica lo verificado** | Ambas fases usan **el mismo** archivo de config; solo cambia `SKIP_PACKAGE_UPLOAD` | `diff` generado vs publicado: **idéntico byte a byte** |
| **No crea tags** | No hay `git tag`, ni `push`, ni acción que los cree | `git tag` y la API de tags: **sin cambios** |
| **No publica manifiestos incompletos** | `pkgcheck` exige que cada URL sea un asset real y que su SHA-256 coincida con el publicado | ZIP local no publicado → **rechazado** |
| **Solo escribe en repositorios autorizados** | Un token por repositorio; `Report destinations` deja el destino en el log | 3 destinos declarados, ninguno más |
| **Falla de forma controlada sin token** | `Require tokens for selected targets` sale con `::error::` antes de generar nada | Falla con mensaje claro, **sin tocar la release** |
| **Falla de forma controlada sin artefacto** | `pkgcheck` valida contra la API, no contra `dist/` | ZIP de snapshot → **falla** |

**El orden importa:** validar el tag → fotografiar la release → exigir
tokens → generar → verificar contra lo publicado → comprobar que la
release no cambió. Así, si algo falla, es antes de escribir fuera.

#### El paso que lo demuestra

```yaml
- name: Assert release unchanged
  run: |
    gh release view "$TAG" --json assets --jq '.assets[] | "\(.name) \(.size)"' | sort > /tmp/after/assets.txt
    gh release download "$TAG" --pattern checksums.txt --dir /tmp/after
    diff -u /tmp/before/assets.txt /tmp/after/assets.txt
    diff -u /tmp/checksums.before /tmp/after/checksums.sorted
```

Si publicar manifiestos hubiera tocado la release de cualquier forma, el
workflow se detendría con `::error::`.

#### Verificación contra la API, no contra `dist/`

`pkgcheck` consulta `GET /repos/{repo}/releases/tags/{tag}` y construye un
mapa de assets publicados. Para cada URL del manifiesto:

1. Exige que empiece por
   `https://github.com/<repo>/releases/download/<tag>/`, así que una URL
   de **otro tag** no pasa.
2. Exige que el nombre del archivo **exista** en la lista de assets.
3. Descarga el asset y compara el **SHA-256** con el del manifiesto.

Esto es lo que impide el error que motivó esta revisión: un ZIP generado
en local puede tener el hash correcto **y aun así no existir** en la
release. Validar solo contra `dist/` habría dado el manifiesto por bueno.

Los tokens se recomienda crearlos como *fine-grained PAT* limitados a
cada repositorio. **Ningún token va en el código ni en la configuración.**

---

## 6. Pruebas

### Ejecutadas y verificadas

| Prueba | Resultado |
|---|---|
| `goreleaser check` | ✅ `1 configuration file(s) validated` |
| Snapshot con los 3 gestores | ✅ 3 manifiestos WinGet + Cask + Scoop |
| ZIP de Windows generado | ✅ `mobatui_<ver>_windows_amd64.zip` |
| Manifiestos WinGet vs esquema oficial `1.12.0` | ✅ **3/3 validan** |
| JSON de Scoop | ✅ válido, `bin: [mobatui.exe]` |
| Hash del ZIP en Scoop y WinGet | ✅ coincide con el artefacto real |
| Hashes del Cask (arm64/amd64) | ✅ coinciden con la release real v0.2.1 |
| **Caso negativo:** hash corrupto | ✅ **detectado** |
| **Caso negativo:** URL de otro tag | ✅ **detectado** |
| **Caso negativo:** ZIP local no publicado | ✅ **detectado** |
| **Caso negativo:** artefacto con hash alterado | ✅ **detectado** |
| Release sin cambios tras publicar manifiestos | ✅ assets y checksums idénticos |
| `actionlint` en los 4 workflows | ✅ sin errores |
| Release sin variables de entorno | ✅ no falla (con `--skip`) |
| Identificador disponible en winget-pkgs | ✅ 404 |

El verificador `cmd/pkgcheck` (etiqueta `packages`, sin dependencias
nuevas) descarga cada URL y compara el SHA-256 real. Demostrado contra la
release v0.2.1 publicada: descargó y validó `mobatui_0.2.1_darwin_arm64.tar.gz`
(3.707.745 bytes) y `darwin_amd64` (4.007.907 bytes), y **rechazó** un
hash alterado.

### NO ejecutadas — límites declarados

| Prueba | Por qué |
|---|---|
| Instalar con `winget install` | **El paquete no está publicado.** No se puede probar sin enviarlo a Microsoft |
| Instalar con `brew install --cask` | `brew` y `ruby` no están instalados en esta máquina (Linux) |
| Instalar con `scoop install` | Scoop es solo Windows; no hay runner Windows en esta máquina |
| Ejecutar el Cask en macOS real | Sin acceso a macOS |
| `rpm -qip` | `rpm` no instalado |

**No declaro ningún gestor como "disponible" ni "probado en instalación
real".** Lo verificado es que los manifiestos son válidos, apuntan a
artefactos existentes y sus hashes coinciden.

Para probar la instalación real hace falta, después de publicar:
un runner Windows (GitHub Actions) para WinGet y Scoop, y uno macOS para
Homebrew.

---

## 7. Integración con GoReleaser: qué se descarta

GoReleaser **puede** actualizar los manifiestos dentro del propio
workflow de release. Se ha decidido **no hacerlo**:

| Opción | Veredicto |
|---|---|
| Todo en `release.yml` | ❌ Un fallo de WinGet impide publicar binarios |
| `skip_upload` con variable de entorno | ❌ Aborta si la variable falta |
| `--skip` por línea de comandos | ✅ **Elegido**: explícito y a prueba de fallos |
| Generar manifiestos en el repo principal | ❌ Duplicaría la fuente de verdad |

**No hace falta ningún script propio de generación.** GoReleaser ya
escribe los tres formatos; el único código nuevo es el *verificador*.

---

## 8. Coste de mantenimiento

| Tarea | Frecuencia | Esfuerzo |
|---|---|---|
| Revisión del PR de WinGet en Microsoft | Cada release | 5–30 min (humano, no automatizable) |
| Actualizar manifiestos de tap/bucket | Cada release | 1 clic (`workflow_dispatch`) |
| Subir versión de GoReleaser | 2–3 veces al año | `goreleaser check` + snapshot |
| Atender un manifiesto roto | Ocasional | Cambio de una línea |
| Rotar tokens | Anual | 10 min × 3 tokens |

**Ventaja de este diseño:** el trabajo por release es un clic, y si algo
falla el usuario sigue teniendo los binarios de GitHub Releases.

**Riesgo de mantenimiento real:** que el Cask de Homebrew deje de
funcionar si Apple endurece Gatekeeper. Mitigación: firmar y notarizar.

---

## 9. Orden recomendado de publicación

1. **Homebrew y Scoop primero** (el mismo día, tras v0.2.2).
   - No dependen de revisión externa: creas el repo, ejecutas el workflow
     y funciona.
   - Permiten validar el flujo completo con riesgo bajo.
   - Scoop solo requiere el ZIP, que ya estará en la release.

2. **WinGet después**, cuando 1 y 2 lleven una release estable.
   - Requiere PR a Microsoft y revisión humana (días).
   - Un primer envío con errores retrasa futuras publicaciones.
   - El manifiesto debe llevar la versión **ya publicada y estable**.

3. **Chocolatey al final**, como se indicó.
   - Exige moderación propia y una cuenta con reputación.
   - Su valor añadido sobre Scoop/WinGet es pequeño para este proyecto.

**Prerrequisito de todo:** publicar una release con el `.zip` de Windows.
Es decir, la primera release que use esta configuración (v0.2.2).

---

## 10. Riesgos y limitaciones

| Riesgo | Impacto | Mitigación |
|---|---|---|
| **Binario sin firmar en macOS** | Gatekeeper lo bloquea | `xattr -dr com.apple.quarantine` en el Cask; a medio plazo, firmar y notarizar |
| **v0.2.1 no tiene ZIP** | Los gestores no pueden publicarse para esa versión | **El verificador lo impide**: rechaza el manifiesto. Esperar a v0.2.2 |
| **Revisión de Microsoft** | El paquete tarda días en aparecer | Enviar tras una release estable; `draft: true` permite revisar antes |
| **Tokens con permisos de escritura** | Un token filtrado permitiría escribir en el tap/bucket | Fine-grained PAT, un token por repositorio, rotación anual |
| **Nombre `Eion21.MOBATUI` reservado por otro** | WinGet rechazaría el paquete | Comprobado libre hoy; si se ocupara, usar `Eion21.Mobatui` |
| **Homebrew Cask sin `livecheck`** | No detecta versiones nuevas solo | Es lo normal en Casks generados; el workflow lo actualiza |
| **Scoop no oficial** | No está en el bucket `main` de Scoop | Requiere instalar el bucket propio del usuario; es lo previsto |
| **`mobatui-render` no se distribuye** | Un usuario no puede instalarlo por gestor | Es una herramienta interna; intencionado |

---

## 11. Qué NO se ha hecho

- ❌ No se creó ningún repositorio externo.
- ❌ No se envió nada a `microsoft/winget-pkgs`.
- ❌ No se publicó ningún manifiesto.
- ❌ No se creó ningún tag ni release.
- ❌ No se modificó ninguna release existente.
- ❌ No se instaló ni reemplazó el ejecutable del sistema.

Todo lo anterior requiere autorización explícita.

---

## 12. Seguridad de GoReleaser y del toolchain

### Versión de GoReleaser

| Versión | Fecha | Estado |
|---|---|---|
| **v2.19.0** | 2026-10-22 | **En uso**. Validada con `check`, snapshot y pruebas de publicación |
| v2.20.0 / v2.20.1 / v2.20.2 | nov 2026 | Estables, **no adoptadas** |

**Por qué no se actualiza todavía:** v2.20.x exige **Go ≥ 1.27.4** y este
entorno tiene **1.27.3** con `GOTOOLCHAIN=local`. Instalarla falla:

```
requires go >= 1.27.4 (running go 1.27.3; GOTOOLCHAIN=local)
```

**No se actualiza a ciegas.** El procedimiento, cuando haya Go 1.27.4+:

1. `goreleaser check` con la versión nueva (detecta deprecaciones).
2. Snapshot completo con los tres gestores.
3. `pkgcheck` contra una release publicada.
4. Comparar los manifiestos generados con los de v2.19.0.
5. Solo entonces cambiar la versión fijada en los workflows.

### Tratamiento de credenciales

Verificado en el código de v2.19.0:

| Aspecto | Comportamiento |
|---|---|
| Tokens en la configuración | Solo por `{{ .Env.VAR }}`, nunca literales |
| Permisos del workflow | `contents: read`; la escritura va por tokens externos |
| Clave SSH | `private_key` se escribe en un archivo temporal y se borra (`defer cleanup`) |
| Alcance | Un token por repositorio destino, no uno global |

**No se encontró ningún advisory de seguridad para GoReleaser** en la API
de GitHub (0 resultados).

### Vulnerabilidad en el toolchain de Go

`govulncheck ./...` encontró **una** vulnerabilidad:

```
GO-2026-6859 · net/http@go1.27.3 · corregida en go1.27.4
HTTP/1 server crash due to MaxBytesReader header race
```

**Evaluación de impacto:** afecta a `MaxBytesReader`, que se usa en el
lado **servidor** HTTP. MOBA TUI es **cliente**: no abre puertos ni usa
`MaxBytesReader` (verificado: 0 apariciones en el código). **La
vulnerabilidad no es explotable en este binario.**

Aun así, se recomienda subir `go 1.27.4` en `go.mod` en la próxima
release, por dos motivos: elimina el hallazgo de `govulncheck` y es
requisito para adoptar GoReleaser v2.20.x.

**No se modificó `go.mod` en este trabajo**: cambiarlo obliga a
regenerar la release y no forma parte del alcance de empaquetado.

---

## 13. Publicación parcial: comportamiento y recuperación

### No es atómica

`--fail-fast` **no** hace la publicación atómica. Evita que se intente el
siguiente gestor tras un error, pero **no revierte** lo ya escrito.

Verificado con Homebrew apuntando a un repositorio válido y Scoop a uno
inexistente:

```
• pushing  repository=.../hb.git  name=homebrew-tap file=Casks/mobatui.rb
⨯ release failed after 20s
  │ scoop manifests: failed to publish artifacts: failed to clone ...
EXIT=1
```

Resultado: Homebrew **quedó publicado** (2 commits) y Scoop no recibió
nada. El job termina en error, sin declarar éxito.

### Recuperación

La reejecución completa **falla**: Homebrew no tiene cambios que
commitear y el paso de Scoop no llega a ejecutarse.

```
homebrew cask: git: failed to commit "homebrew-tap": "commit -m ..." failed
```

**Procedimiento correcto** — reejecutar omitiendo lo ya publicado:

```yaml
targets: scoop        # o homebrew, según lo que quedó pendiente
```

Internamente el workflow traduce eso a `--skip=homebrew`, y entonces:

```
• pushing  repository=.../sc.git  name=scoop-bucket file=bucket/mobatui.json
• release succeeded
```

Verificado: Homebrew se quedó en 2 commits (**sin duplicar**) y Scoop pasó
de 1 a 2.

### Idempotencia

Con los manifiestos ya publicados, **no se crea un commit nuevo**: el
contenido es idéntico y GoReleaser no encuentra cambios que commitear
(verificado: 2 → 2 commits).

Por eso la reejecución de un gestor ya publicado **falla en lugar de
duplicar**. Es un fallo controlado, no corrupción.

### Lo que hay que saber antes de ejecutarlo

| Situación | Resultado |
|---|---|
| Todo correcto | Éxito, un commit por repositorio |
| Falla el segundo gestor | El primero queda publicado; job en error |
| Se reejecuta todo | Falla en el primero ya publicado (nada que commitear) |
| Se reejecuta con `--skip` del ya publicado | Completa el resto, sin duplicados |

**No existe rollback automático.** Si un manifiesto quedó mal, se corrige
con otro commit en ese repositorio.
