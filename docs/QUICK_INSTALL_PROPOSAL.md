# Propuesta: sección «Quick install» para los README

**No aplicar todavía.** Los tres gestores están sin publicar. Mostrar
`winget install` o `brew install` en el README haría que los usuarios
ejecutaran comandos que fallan.

Cuando cada gestor esté realmente publicado, sustituir la sección
`## Requisitos e instalación` (README.md) y `## Requirements and
installation` (README.en.md) por la versión correspondiente.

---

## Nivel 0 — ahora (sin gestores publicados)

Es lo que debería estar en el README hoy. Solo GitHub Releases y Go.

### README.md

```markdown
## Requisitos e instalación

- **Go 1.27** o superior para compilar desde el código fuente.
- Una **terminal con soporte Truecolor** (Kitty, Alacritty, WezTerm,
  Ghostty, Windows Terminal, iTerm2, etc.).
- **Plataformas:** Linux es la plataforma principal; macOS y Windows
  están soportados en código y CI.

### Binarios precompilados

Descarga el archivo de tu plataforma desde
[Releases](https://github.com/Eion21/mobatui/releases/latest):

| Plataforma | Archivo |
| --- | --- |
| Linux x86-64 | `mobatui_<versión>_linux_amd64.tar.gz` |
| Linux ARM64 | `mobatui_<versión>_linux_arm64.tar.gz` |
| macOS Apple Silicon | `mobatui_<versión>_darwin_arm64.tar.gz` |
| macOS Intel | `mobatui_<versión>_darwin_amd64.tar.gz` |
| Windows x86-64 | `mobatui_<versión>_windows_amd64.zip` |

```bash
tar -xzf mobatui_*_linux_amd64.tar.gz
./mobatui
```

En Debian/Ubuntu también hay paquetes `.deb`, y `.rpm` para Fedora/RHEL.

### Desde el código fuente

```bash
go install github.com/Eion21/mobatui@latest
mobatui
```
```

### README.en.md

```markdown
## Requirements and installation

- **Go 1.27** or newer to build from source.
- A **Truecolor terminal** (Kitty, Alacritty, WezTerm, Ghostty,
  Windows Terminal, iTerm2, etc.).
- **Platforms:** Linux is the primary platform; macOS and Windows are
  supported in code and CI.

### Prebuilt binaries

Download the archive for your platform from
[Releases](https://github.com/Eion21/mobatui/releases/latest):

| Platform | File |
| --- | --- |
| Linux x86-64 | `mobatui_<version>_linux_amd64.tar.gz` |
| Linux ARM64 | `mobatui_<version>_linux_arm64.tar.gz` |
| macOS Apple Silicon | `mobatui_<version>_darwin_arm64.tar.gz` |
| macOS Intel | `mobatui_<version>_darwin_amd64.tar.gz` |
| Windows x86-64 | `mobatui_<version>_windows_amd64.zip` |

```bash
tar -xzf mobatui_*_linux_amd64.tar.gz
./mobatui
```

`.deb` packages are available for Debian/Ubuntu and `.rpm` for
Fedora/RHEL.

### From source

```bash
go install github.com/Eion21/mobatui@latest
mobatui
```
```

---

## Nivel 1 — con Homebrew y Scoop publicados

Añadir **solo** los bloques de los gestores que estén ya en su catálogo.
Aquí se muestran los dos, porque se propone publicarlos primero.

### README.md (bloque nuevo, tras la tabla de binarios)

```markdown
### Instalación rápida

**macOS (Homebrew):**

```bash
brew install --cask eion21/tap/mobatui
```

> **Primer arranque:** el binario no está firmado ni notarizado por Apple,
> así que macOS mostrará un aviso de seguridad. Ábrelo una vez desde
> *Ajustes del Sistema → Privacidad y seguridad → «Abrir igualmente»*.
> No hace falta desactivar Gatekeeper.

**Windows (Scoop):**

```powershell
scoop bucket add eion21 https://github.com/Eion21/scoop-bucket
scoop install eion21/mobatui
```

**Sin gestores de paquetes:** descarga el binario de
[Releases](https://github.com/Eion21/mobatui/releases/latest) (ver tabla
arriba) y ejecútalo. No requiere instalación.
```

### README.en.md

```markdown
### Quick install

**macOS (Homebrew):**

```bash
brew install --cask eion21/tap/mobatui
```

> **First run:** the binary is not signed or notarized by Apple, so macOS
> will show a security warning. Open it once from *System Settings →
> Privacy & Security → "Open Anyway"*. Disabling Gatekeeper is not needed.

**Windows (Scoop):**

```powershell
scoop bucket add eion21 https://github.com/Eion21/scoop-bucket
scoop install eion21/mobatui
```

**No package manager:** download the binary from
[Releases](https://github.com/Eion21/mobatui/releases/latest) (table
above) and run it. No installation required.
```

---

## Nivel 2 — con WinGet publicado

WinGet va **primero** en Windows por ser el gestor nativo de Windows 10/11.
Se añade como primera opción cuando el paquete esté en el catálogo.

### README.md

```markdown
### Instalación rápida

**Windows (WinGet):**

```powershell
winget install Eion21.MOBATUI
```

**macOS (Homebrew):**

```bash
brew install --cask eion21/tap/mobatui
```

> **Primer arranque:** macOS mostrará un aviso de seguridad porque el
> binario no está notarizado. Ábrelo una vez desde *Ajustes del Sistema →
> Privacidad y seguridad → «Abrir igualmente»*.

**Windows (Scoop):**

```powershell
scoop bucket add eion21 https://github.com/Eion21/scoop-bucket
scoop install eion21/mobatui
```

**Sin gestores de paquetes:** descarga el binario de
[Releases](https://github.com/Eion21/mobatui/releases/latest) (ver tabla
arriba) y ejecútalo. No requiere instalación.

Actualizar: `winget upgrade Eion21.MOBATUI`,
`brew upgrade --cask mobatui` o `scoop update mobatui`.
```

### README.en.md

```markdown
### Quick install

**Windows (WinGet):**

```powershell
winget install Eion21.MOBATUI
```

**macOS (Homebrew):**

```bash
brew install --cask eion21/tap/mobatui
```

**Windows (Scoop):**

```powershell
scoop bucket add eion21 https://github.com/Eion21/scoop-bucket
scoop install eion21/mobatui
```

**No package manager:** download the binary from
[Releases](https://github.com/Eion21/mobatui/releases/latest) and run it.

Upgrade: `winget upgrade Eion21.MOBATUI`,
`brew upgrade --cask mobatui` or `scoop update mobatui`.
```

---

## Reglas para no mentir en el README

1. **Un comando solo aparece si funciona hoy.** Si el paquete está en
   revisión en Microsoft, no se escribe.
2. **GitHub Releases siempre presente**, como método alternativo. Nunca
   se deja al usuario sin una forma de instalar.
3. **La tabla de archivos se actualiza con cada release**: nombres y
   plataformas salen del `checksums.txt`.
4. **No se promete firma ni notarización.** En macOS se documenta el
   comando de instalación **y el aviso de Gatekeeper del primer arranque**.
   No se oculta esa fricción ni se instruye a desactivar protecciones.
5. **Sin cifras de usuarios ni de popularidad** que no se puedan
   respaldar.

---

## Nota sobre el archivo de Windows

El nivel 0 y el nivel 1 muestran `mobatui_<versión>_windows_amd64.zip`,
que **todavía no existe**: v0.2.1 publicó un `.tar.gz`. El `.zip` aparecerá
en la primera release que use `format_overrides` (v0.2.2 o posterior).

Hasta entonces, el README debe seguir diciendo `.tar.gz` para Windows.
