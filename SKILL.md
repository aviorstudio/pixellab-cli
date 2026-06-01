---
name: pixellab-cli
description: Use pxlb, the PixelLab v2 API CLI, for generating PixelLab images, objects, characters, animations, tilesets, checking account balance, downloading outputs, or automating PixelLab asset generation.
---

# PixelLab CLI

`pxlb` is a thin Go CLI for the PixelLab v2 API. It exposes API routes directly, maps request fields to flags, supports async job polling, and can save returned images or binary downloads.

Use this skill when the user wants to run PixelLab API operations from the command line or automate PixelLab asset generation.

## Command Model

Use built-in help to inspect available routes and CLI behavior before guessing flags:

```bash
pxlb --help
```

Use API routes without the leading slash:

```bash
pxlb <api-route> [flags]
```

Examples:

```bash
pxlb balance
pxlb generate-image-v2 --description "crystal sword" --image-size 128x128 --wait --out ./out
pxlb create-1-direction-object --description "wooden barrel" --size 128 --view sidescroller
pxlb objects/<object_id>/animations --animation-description "heavy sword slash" --frame-count 16 --wait
```

Route and method rules:

- Path parameters go directly in the route, such as `objects/abc123`; do not pass duplicate ID flags for path params.
- The HTTP method is inferred for unique routes.
- `GET`/`DELETE` route collisions default to safe `GET`.
- Use `--http-method delete` for destructive deletes.
- Use `--body-json` as an escape hatch for complex request bodies.
- Use `--wait` to poll async background jobs.
- Use `--out <path>` to write binary or image outputs.

## Common Workflows

Check account balance:

```bash
pxlb balance
```

Generate and download an image:

```bash
pxlb generate-image-v2 --description "pixel art crystal sword" --image-size 128x128 --wait --out ./out
```

Create and inspect an object:

```bash
pxlb create-1-direction-object --description "wooden barrel" --size 128
pxlb objects/<object_id>
```

Animate an object:

```bash
pxlb objects/<object_id>/animations --animation-description "heavy sword slash" --frame-count 16 --wait
```

Delete an object explicitly:

```bash
pxlb objects/<object_id> --http-method delete
```

## PixelLab Asset Guidance

- Prefer `16` frames for complex combat actions, then downselect if the target game needs fewer frames.
- Describe animation beats explicitly: idle, anticipation, strike, follow-through, recovery.
- Avoid vague combat prompts like `attack`; specify body pose, weapon motion, and silhouette.
- Avoid `--enhance-prompt` when precise combat intent matters.
- Words like `dramatic`, `impact`, `slash arc`, `energy`, and `burst` often introduce VFX.
- For cleaner physical attacks, specify `no magic effects`, `no glowing slash trail`, `no projectile`, and `pose only` when appropriate.

## References

- PixelLab API docs: `https://api.pixellab.ai/v2/docs`.
