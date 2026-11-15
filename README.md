# MOBA TUI

A terminal UI for **Deadlock** and **League of Legends** that turns real
match data into item builds, skill orders and stats — readable at a glance
during a game.

Built in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea) and
[Lip Gloss](https://github.com/charmbracelet/lipgloss).

---

## What it does

**Deadlock** (via [deadlock-api.com](https://deadlock-api.com))

- Item builds assembled from real purchase sequences, not just popularity:
  a hybrid generator combines per-item statistics with a progression engine
  that respects upgrade chains, component consumption and inventory limits.
- Builds split into **EARLY / MID / LATE**, plus **extensions** for long
  games and **situational** items grouped by function (anti-heal, bullet
  defence, spirit defence, crowd-control resistance, survivability, extra
  damage).
- Every item carries a confidence label (high / moderate / low) derived from
  sample size, statistical uncertainty and consistency — never presented as
  a win probability.
- Skill orders mapped to the hero's **real keys** using the game's own
  metadata (`signature1..4`), including heroes whose abilities change form
  when transformed (e.g. Silver: 7 statistical records collapse into 4 real
  slots).
- Hero win-rate trend, matchups and synergies.

**League of Legends** (via [lolalytics.com](https://lolalytics.com))

- Item builds, rune pages, skill order and summoner spells.
- Items ranked by the **lower bound of the Wilson score**, not raw win rate,
  so three-game samples do not outrank 40,000-game ones.
- Champion art rendered as ASCII in the terminal.

**Both games**

- Everything goes through a local on-disk cache with TTL, degrading to
  bundled fixtures when offline.
- Game monitor (`m`): detects the running game, and in LoL the champion in
  the current match via the local Live Client Data API.

---

## Requirements

- Go 1.27 or newer to build from source.
- A terminal with truecolor support for item and hero art.
- Linux is the primary platform. macOS and Windows are supported by the
  code and CI, but have had less manual testing.

---

## Install

```bash
go install github.com/Eion21/mobatui@latest
```

Or build from a clone:

```bash
git clone https://github.com/Eion21/mobatui
cd mobatui
go build -o mobatui .
./mobatui
```

Runtime cache lives in your user cache directory (`~/.cache/mobatui` on
Linux) and preferences in your user config directory
(`~/.config/mobatui/config.json`). Nothing is written inside the repository.

### Flags

| Flag | Default | Meaning |
|---|---|---|
| `-data` | user cache dir | Runtime cache directory |
| `-offline` | `false` | No network: cache and fixtures only |
| `-refresh` | `false` | Invalidate static cache before starting |
| `-locale` | `es_ES` | Data Dragon language for LoL |
| `-ugg` | `testdata/ugg` | u.gg dump directory |
| `-deadlock` | `testdata/deadlock/builds` | Deadlock fixture directory |

---

## Controls

| Key | Action |
|---|---|
| `1` / `2`, `←` `→` | Pick game |
| `enter` | Open champion / reuse loaded session |
| `r` | Force reload (or cycle role in detail) |
| `R` | Cycle role backwards |
| `tab`, `1`-`5` | Switch tabs |
| `←↑↓→`, `hjkl` | Move selection |
| `enter` | Item detail popup |
| `esc` | Close popup / cancel filter / go back |
| `u` | Fold or show upgrade cards |
| `a` | Cycle portrait mode |
| `m` | Toggle game monitor |
| `?` | Help |
| typing | Filters the list |
| mouse wheel | Scroll |

---

## How the builds work

The generator is deliberately conservative about what it claims.

1. **Observed route** — purchase sequences from real matches, ranked by
   consensus precedence. This is what most players actually buy.
2. **Statistical evaluation** — each candidate is scored on popularity,
   phase-adjusted frequency, adjusted performance, sample size, statistical
   uncertainty and consistency across the sample. A Beta-Binomial posterior
   is used alongside the Wilson lower bound; neither is treated as causal.
3. **Progression engine** — validates that a proposed sequence is actually
   purchasable: upgrade chains must exist in the catalogue, components are
   consumed, inventory capacity is respected, no cycles.
4. **Hybrid build** — combines the above into a sequence that is both
   statistically supported and mechanically valid, with a reason recorded
   for every item included or excluded.

A high win rate is **not** treated as evidence that an item causes wins.
Items bought while already ahead show inflated win rates; the scoring
accounts for this rather than rewarding it.

### Confidence

Confidence is a label, not a probability:

- **high** — large sample, narrow uncertainty, consistent across the sample.
- **moderate** — usable, but the sample does not support strong claims.
- **low** — shown for completeness; treat as a hint.

---

## Cache and freshness

All network responses are cached on disk with a TTL and served stale on
failure rather than breaking the app. The status bar shows whether data came
from network, cache, stale cache or bundled fixtures.

Reopening a hero you already visited reuses the computed result for that
session; it does not re-fetch or recompute.

No request rate is increased above what the upstream APIs allow. The app is
designed to be run by one person, not as a service.

---

## Known limitations

- **Builds are statistical consensus, not optimal play.** They describe what
  wins in the data, not what will win your next game.
- **No causal claims.** A higher win rate on an item does not mean buying it
  improves your results.
- **Sample sizes vary.** Some heroes have thousands of matches, others a few
  hundred. Confidence labels reflect this; low-confidence items should be
  read as suggestions.
- **Hero coverage.** 65 heroes are in the catalogue; 44 have enough cached
  match data to build a route. The rest show general statistics without a
  purchase order. Two heroes in development have no published ability kit.
- **Situational item labels** come from verifiable item properties (name and
  category), not from matchup analysis. The app does not claim an item
  counters a specific hero.
- **No real-terminal visual validation** has been recorded for the latest
  layout changes; verification is automated tests plus text renders at fixed
  sizes.
- **League of Legends data** is decoded from Lolalytics' server-rendered
  HTML. If their markup changes, parsing breaks until updated.

---

## Data sources and credits

- **Deadlock** data: [deadlock-api.com](https://deadlock-api.com) —
  community-run API. Hero, item and match data belong to Valve.
- **League of Legends** data: [lolalytics.com](https://lolalytics.com) for
  statistics, [Data Dragon](https://developer.riotgames.com/docs/lol#data-dragon)
  for names and images.
- **Live Client Data API** (local, `127.0.0.1:2999`) for the in-game
  champion, when League is running.

Deadlock is a trademark of Valve Corporation. League of Legends and Riot
Games are trademarks of Riot Games, Inc. This project is not affiliated with,
endorsed by, or sponsored by either company. Game assets are used for
identification purposes only.

---

## Development

```bash
go test ./...      # full suite, no network required
go vet ./...
gofmt -l .         # must be empty
go run ./cmd/render deadlock   # text render, no TUI needed
```

Tests use bundled fixtures and a local HTTP server; they do not hit the
public game APIs. `cmd/render` dumps a view to stdout so layout changes can
be checked without a terminal.

### Layout

```
internal/deadlock/    Deadlock API client, route building, evaluation, hybrid generator
internal/progression/ Item catalogue, upgrade chains, inventory simulation
internal/lolalytics/  LoL statistics client
internal/ddragon/     LoL static data and images
internal/clean/       Wilson scoring and build cleaning
internal/ui/          Bubble Tea views, cards, navigation
internal/cache/       On-disk cache with TTL and stale-if-error
```

---

## License

MIT — see [LICENSE](LICENSE). Game data and assets remain the property of
their respective owners and are not covered by this license.
