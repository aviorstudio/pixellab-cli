# pxlb

`pxlb` is a thin Go CLI for the [PixelLab v2 API](https://api.pixellab.ai/v2/docs). It exposes PixelLab routes directly, keeps request fields as simple flags, supports async job polling, and can save returned images or binary downloads.

## Quick Start

```bash
make build
./bin/pxlb --help
./bin/pxlb --version
```

Install to `$(go env GOPATH)/bin/pxlb`:

```bash
make install
```

Set your API key:

```bash
export PIXELLAB_API_KEY="..."
pxlb balance
```

You can also put `PIXELLAB_API_KEY=...` in a `.env` file in the directory where you run `pxlb`, or pass `--token <key>` for a single command.

## Command Model

Use the API route without the leading slash:

```bash
pxlb <api-route> [flags]
```

Examples:

```bash
pxlb balance
pxlb create-image-pixen --description "cute dragon" --image-size 128x128 --out ./out
pxlb generate-image-v2 --description "crystal sword" --image-size 128x128 --wait --out ./out
pxlb objects/<object_id>/animations --animation-description "heavy sword slash" --frame-count 16 --wait
```

`pxlb` infers the HTTP method from the route:

- Unique routes use their only supported method.
- Path parameters go directly in the route, such as `objects/abc123`; do not pass the same id again as `--object-id`.
- `GET`/`DELETE` collisions default to safe `GET`.
- Use `--http-method delete` for destructive delete routes.
- `/tilesets` has both `GET` and `POST`; body flags or `--body-json` select `POST`, otherwise it lists tilesets with `GET`.

Collision examples:

```bash
pxlb objects/<object_id>
pxlb objects/<object_id> --http-method delete
pxlb tilesets --limit 50 --offset 0
pxlb tilesets --lower-description "ocean" --upper-description "beach"
```

## Built-In Help

Show all routes with one-sentence descriptions:

```bash
pxlb --help
```

Show build metadata:

```bash
pxlb --version
```

## Global Flags

| Flag | Purpose |
|---|---|
| `--token <key>` | PixelLab API key. Overrides `PIXELLAB_API_KEY` and `.env`. |
| `--base-url <url>` | API base URL. Defaults to `https://api.pixellab.ai/v2`. |
| `--http-method <method>` | Override inferred method for colliding routes. Usually only needed for delete. |
| `--json` | Print raw JSON response. |
| `--out <path>` | Save binary responses or returned base64 images. |
| `--wait` | Poll returned background jobs until completion. |
| `--poll-interval <duration>` | Polling interval for `--wait`, for example `5s`. |
| `--timeout <duration>` | Max wait duration, for example `10m`. |
| `--body-json <json-or-file>` | Send raw JSON from an inline JSON string or file. |
| `--quiet` | Suppress normal JSON output. |

## Flag Conventions

Request body fields become kebab-case flags:

```bash
pxlb create-image-pixen \
  --description "crystal sword" \
  --image-size 128x128 \
  --outline "selective outline" \
  --no-background \
  --seed 123
```

Common conversions:

- API `snake_case` fields become CLI `--kebab-case` flags.
- Size values use `WIDTHxHEIGHT`, such as `128x128`.
- Image flags accept local file paths and are encoded as PixelLab `Base64Image` objects.
- Repeated flags create arrays, such as `--tag enemy --tag boss`.
- Complex request bodies can always use `--body-json`.

Raw JSON escape hatch:

```bash
pxlb objects/<object_id>/animations \
  --body-json ./animation-request.json \
  --wait
```

## Async Jobs And Output

Many generation routes return a background job. Add `--wait` to poll until completion:

```bash
pxlb generate-image-v2 \
  --description "crystal sword" \
  --image-size 128x128 \
  --wait \
  --out ./out
```

Check a job manually:

```bash
pxlb background-jobs/<job_id>
```

Save binary downloads:

```bash
pxlb characters/<character_id>/zip --out character.zip
```

## Common Workflows

### Account

```bash
pxlb balance
```

### Image Generation

```bash
pxlb create-image-pixen --description "cute dragon" --image-size 128x128 --out ./out
pxlb create-image-bitforge --description "ancient shield" --image-size 128x128
pxlb create-image-pixflux --description "fire spell icon" --image-size 128x128
pxlb generate-image-v2 --description "crystal sword" --image-size 128x128 --wait --out ./out
pxlb generate-with-style-v2 --style-image style.png --description "wooden chest" --image-size 128x128 --wait
```

### Image Editing

```bash
pxlb remove-background --image input.png --image-size 128x128 --out out.png
pxlb image-to-pixelart --image input.png --image-size 512x512 --output-size 128x128
pxlb resize --description "resize cleanly" --reference-image input.png --reference-image-size 64x64 --target-size 128x128
pxlb inpaint-v3 --description "golden crown" --inpainting-image image.png --mask-image mask.png
pxlb edit-image --image input.png --image-size 128x128 --description "make it golden" --width 128 --height 128
```

### Objects

```bash
pxlb create-1-direction-object --description "wooden barrel" --size 128 --view sidescroller
pxlb create-8-direction-object --description "stone fountain" --size 128 --view "low top-down"
pxlb objects --limit 50 --offset 0
pxlb objects/<object_id>
pxlb objects/<object_id> --http-method delete
pxlb objects/<object_id>/tags --tag barrel --tag prop
```

Review-pack object workflow:

```bash
pxlb create-1-direction-object --description "forest mushroom" --size 128 --wait
pxlb objects/<object_id>/select-frames --index 0 --index 3 --common-tag props
pxlb objects/<object_id>/dismiss-review
```

Object states:

```bash
pxlb objects/<object_id>/states --edit-description "make it golden"
```

### Object Animation

```bash
pxlb objects/<object_id>/animations \
  --animation-description "heavy sword slash" \
  --display-name attack \
  --frame-count 16 \
  --wait
```

For combat animations, we found that 16-frame v3 generations plus manual downselection usually work best. See [Attack Animation Experiments](docs/attack-animation-experiments.md) for prompt lessons and examples.

### Characters

```bash
pxlb create-character-v3 --description "blue wizard" --image-size 64x64
pxlb create-character-pro --description "armored knight" --image-size 96x96
pxlb create-character-with-4-directions --description "goblin" --image-size 64x64
pxlb create-character-with-8-directions --description "goblin" --image-size 64x64
pxlb characters --limit 50 --offset 0
pxlb characters/<character_id>
pxlb characters/<character_id>/zip --out character.zip
pxlb characters/<character_id> --http-method delete
```

Character states and animations:

```bash
pxlb create-character-state --character-id <uuid> --edit-description "wearing red armor"
pxlb animate-character --character-id <uuid> --mode v3 --action-description "walking" --frame-count 8
pxlb characters/animations --character-id <uuid> --action-description "walking"
```

### Animation API Routes

```bash
pxlb animate-with-text-v3 --first-frame idle.png --action "heavy sword slash" --frame-count 16
pxlb animate-with-text-v2 --reference-image sprite.png --reference-image-size 128x128 --action "attack" --image-size 128x128
pxlb animate-with-skeleton --reference-image sprite.png --image-size 64x64 --skeleton-keypoints-json poses.json
pxlb interpolation-v2 --start-image start.png --end-image end.png --action "slash" --image-size 128x128
pxlb edit-animation-v2 --description "add glow" --frames-json frames.json --image-size 128x128
```

### Tiles And Tilesets

```bash
pxlb create-isometric-tile --description "grass on dirt" --image-size 32x32
pxlb isometric-tiles --limit 50 --offset 0
pxlb isometric-tiles/<tile_id>

pxlb create-tiles-pro --description "1). grass 2). dirt 3). stone" --tile-type isometric --tile-size 32
pxlb tiles-pro/<tile_id>

pxlb tilesets --lower-description "ocean" --upper-description "beach"
pxlb tilesets --limit 50 --offset 0
pxlb tilesets/<tileset_id>

pxlb tilesets-sidescroller --lower-description "stone bricks" --transition-description "moss"
pxlb create-tileset-sidescroller --lower-description "stone bricks" --transition-description "moss"
```

### Prompt Enhancement

```bash
pxlb enhance-pixen-prompt --description "dragon" --image-size 128x128
pxlb enhance-character-v3-prompt --description "blue wizard" --image-size 64x64
pxlb enhance-animation-v3-prompt --first-frame idle.png --action "walking"
```

## Documentation

- [Full CLI endpoint map](docs/api-v2-cli-map.md): every PixelLab v2 route, flags, schemas, async behavior, and implementation notes.
- [Attack animation experiments](docs/attack-animation-experiments.md): practical lessons from dialing in readable melee attack frames.
- [PixelLab API docs](https://api.pixellab.ai/v2/docs): upstream API reference.
- [PixelLab OpenAPI spec](https://api.pixellab.ai/v2/openapi.json): upstream machine-readable schema.
- [PixelLab LLM docs](https://api.pixellab.ai/v2/llms.txt): condensed upstream docs for agents and LLMs.

## Development

```bash
make test
make build
./bin/pxlb --help
```

The built binary is written to `bin/pxlb`, and `bin/` is ignored by git.

## Releases

Releases use `v*` tags. The manual GitHub Actions release workflow must run from `main`, accepts a `patch`, `minor`, or `major` bump, starts at `v0.0.1` when no release tags exist, runs Go tests, injects version/build metadata, and builds `pxlb` binaries for Linux, macOS, and Windows with checksums.
