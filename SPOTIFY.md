# Spotify capability notes

Findings from tracing real Yori conversations down through the tool layer
(`go.naturallyfunny.dev/adk/spotify`), the module (`go.naturallyfunny.dev/spotify`,
dev'd at `/Users/ardian/dev/spotify`), and the underlying `zmb3/spotify/v2` client.

The recurring pattern: **Yori tells the user "I can't do X"** when the Spotify Web
API *can* do X. In each case the limit is in our layer (tool prose or an unwired
option), not in Spotify.

---

## 1. Activating an inactive-but-available device

**Yori's claim:** "I can't activate a Spotify device myself; you must open Spotify
on a device first."

**Reality: not an API limitation. The capability exists at every layer below the
tool prose.**

- Spotify Web API supports it two ways:
  - `PUT /me/player/play` with a `device_id` — starts playback on a specific
    device even if it wasn't the active one.
  - `PUT /me/player` (Transfer Playback) — moves/activates the session to another
    device.
- `zmb3/spotify/v2` exposes both: `PlayOpt(opts{DeviceID})` and
  `TransferPlayback(ctx, deviceID, play)`.
- Our module already wires the device_id path: `spotify.Play(ctx, userID,
  deviceID, uri)` sets `opts.DeviceID` (`player.go:78`).
- The adk tool already exposes it: `play` takes a `device_id` param, and
  `my_devices` returns the list to pick from (`adk/spotify/toolset.go:196,218`).

A device only has to be **available** (appear in `my_devices`) — i.e. the Spotify
app/web player is open somewhere. An available device can be inactive and still be
targeted. In the transcript, `my_devices` returned `naturallyfunny` (active) **and**
`Web Player (Safari)` (inactive) — Yori could have called `play(device_id=<web
player>)` to wake it, no user action needed.

**Root cause = behavioral, not missing capability.** The `play` tool description
(`toolset.go:232`) nudges the model to punt:

```
- No active device? Spotify can't place the sound; ask the human to open
  Spotify somewhere, or check my_devices.
```

**Only genuinely-can't case:** `my_devices` is empty (every Spotify app closed →
no available device). Then the user must open Spotify first.

**Fix:**
1. Rewrite the `play` tool prose: on a no-active-device error, call `my_devices`
   and **retry `play` with an available `device_id`** before asking the human.
   Ask the human only when `my_devices` is empty.
2. Optional: add a dedicated `transfer_playback` tool wrapping
   `TransferPlayback` for cleaner "wake an idle available device" semantics.
3. Mirror the same guidance in `internal/instruction.txt`.

Fixes 1–2 live in the published `adk/spotify` module → need a release + version bump.

---

## 2. Playing a specific track *within* a playlist context (keep skip next/prev)

**Yori's claim:** "`play` can only play one track directly, or the whole playlist
from the start. I can't start a playlist at a specific track and keep skip
next/prev in the playlist context."

**Reality: the Spotify API can do exactly this. Our module can't — genuine
missing implementation, not an API limit.**

- Spotify Web API: `PUT /me/player/play` accepts `context_uri` (the playlist)
  **together with** `offset` (either `{position: N}` or `{uri: <track uri>}`).
  Result: playback runs *inside the playlist context*, starting at the chosen
  track, with skip next/prev intact.
- `zmb3/spotify/v2` supports it: `PlayOptions` has `PlaybackOffset`
  (`{Position *int}` or `{URI}`) alongside `PlaybackContext` (`player.go:96`).

**The gap is in our module.** `spotify.Play` (`player.go:78`) does **either** a
context **or** a single-track URI — never both, and never sets `PlaybackOffset`:

```go
if isContextURI(uri) {
    opts.PlaybackContext = &ctxURI   // whole playlist from the start
} else {
    opts.URIs = []spotify.URI{...}   // one detached track, no context
}
```

So today the only two reachable behaviors are exactly the two Yori described.
Playing "Kacamata" from playlist "🌌" with working skip is impossible **through our
module**, even though the API offers it.

**Fix:** extend the module + tool to accept a context + an offset, e.g. `Play(ctx,
userID, deviceID, contextURI, offsetURI, trackURI)` or a small options struct.
When the human picks a track that came from a playlist, call with
`PlaybackContext=<playlist uri>` and `PlaybackOffset.URI=<track uri>`.

- Module change: `go.naturallyfunny.dev/spotify` `player.go` → release + bump.
- Tool change: `adk/spotify` `play` tool needs a `context_uri` (or
  `playlist_uri`) + `offset` param, and description explaining when to use it.
- Instruction: teach Yori that when it played a track it found *inside* a
  playlist, it should pass the playlist as context so skip works.
