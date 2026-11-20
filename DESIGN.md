# Design / UX notes

`fan` is a small tool, but the TUI is still a UI, so this file records the
UX decisions. The bar we aim for is the design ethos of tw93/Mole: quiet,
minimal, glanceable, safe-by-default, and it never makes decisions for the user
silently.

## Principles (borrowed from Mole)

- **Dashboard first, not a menu first.** The main screen shows live state
  (fan, temperature, power, mode) at a glance; actions are secondary.
- **Common/safe actions first, risky ones behind "Advanced".** Quiet is the
  obvious primary action; raw profiles / fan-curve / governor live in Advanced.
- **Show current state, don't make me infer it.** The active mode has a `●`
  marker; temperatures and a health label are colour-coded.
- **Feedback for every action.** A one-line toast confirms what happened;
  errors are shown in red. Temporary actions (max fan) show a countdown and are
  cancellable (Esc).
- **Quiet chrome, single source.** One title bar, no duplicated headers, fixed
  sections, footer help that never overlaps content. Preferences persist.
- **Responsive.** Layout is sized to the terminal; long text is truncated.

## Information architecture

```
Dashboard
├─ Header        ● R9000P Fan ................ HEALTHY (green/amber/red)
├─ Metrics       FAN 2500/2700 rpm [bar]   TEMP 58/52°C   POWER power-saver
├─ Mode (radio)  ● Quiet / ○ Balanced / ○ Performance
├─ Actions       Max fan 30s (momentary) · Advanced ›
└─ Footer        ↑↓ move · ↵ apply · q quit · updated Ns ago

Advanced (esc back)
├─ Platform profile: low-power / balanced / performance / custom
├─ Fan curve: default(EC) / quiet / max
├─ CPU governor: powersave / performance
├─ Diagnostics: show raw values
└─ ‹ Back
```

## Colour semantics

- Temperature: green < 70°C, amber 70–85°C, red ≥ 85°C.
- Header health label: HEALTHY / WARM / HOT from the hottest sensor.
- Active mode marker and success toast: green. Errors: red. Selection: magenta.

## Modes

- **Quiet** = `power-saver` + gentle fan curve (low-temp points lowered, high-temp
  points kept for safety).
- **Balanced** = `balanced` power profile, EC default fan curve.
- **Performance** = `performance` profile.
- **Max fan** is momentary: overrides to full speed with a countdown, then
  restores the previous profile/curve. Esc cancels (still restores).

## Keyboard

`↑/↓` or `j/k` move · `↵` apply · `s`/tick auto-refresh · `esc` back (Advanced)
or quit (Dashboard) · `q` quit.

## Non-interactive

Sub-commands stay stable for scripting: `fan status` (and `--json` planned),
`fan quiet on|off`, `fan max N`, `fan profile <name>`.

## Roadmap

- `fan status --json` for scripts/agents.
- Optional temperature sparkline (only if it stays calm).
- Persist last tab / view preference.
