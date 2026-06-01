# PixelLab v2 CLI Map

This document maps the PixelLab v2 REST API to the planned Go CLI surface. The CLI should mirror the API layout clearly and support every endpoint in the v2 OpenAPI spec.

Source of truth:

- API docs: `https://api.pixellab.ai/v2/docs`
- OpenAPI spec: `https://api.pixellab.ai/v2/openapi.json`
- LLM docs: `https://api.pixellab.ai/v2/llms.txt`

Observed API surface: `61` unique paths, `64` HTTP operations, and `195` schemas.

## Command Model

Expose endpoints directly by HTTP method and API path:

```bash
pixellab get /balance
pixellab post /create-image-pixen --description "cute dragon" --image-size 128x128
pixellab get /objects/{object_id} --object-id <uuid>
pixellab delete /objects/{object_id} --object-id <uuid>
```

Rules:

- First positional arg is the HTTP method: `get`, `post`, `patch`, or `delete`.
- Second positional arg is the exact API path from docs.
- Path parameters may be supplied with named flags, such as `--object-id`, `--character-id`, `--job-id`, `--tileset-id`, or `--tile-id`.
- The CLI should replace `{param}` placeholders before sending the request.
- All request body fields should be available as flags using kebab-case names matching snake_case JSON fields.
- All endpoints should also support `--body-json <path-or-json>` as an escape hatch for full raw request bodies.
- All commands should support `--json` to print raw API responses.

## Global Flags

| Flag | Default | Purpose |
|---|---:|---|
| `--token` | `PIXELLAB_API_KEY` | Bearer token/API key. |
| `--base-url` | `https://api.pixellab.ai/v2` | API base URL. |
| `--json` | `false` | Print raw JSON response. |
| `--out` | empty | Output file or directory for returned images, ZIPs, or downloaded assets. |
| `--wait` | `false` | Poll async jobs until completed when a job id is returned. |
| `--poll-interval` | `5s` | Poll interval for `--wait`. |
| `--timeout` | `0` | Optional max wait duration. |
| `--body-json` | empty | Raw JSON body, either inline JSON or a file path. |
| `--quiet` | `false` | Suppress progress output. |

## Authentication

Most endpoints require:

```http
Authorization: Bearer <token>
```

Unauthenticated endpoints in the spec:

- `GET /llms.txt`
- `GET /characters/{character_id}/zip` reports no security requirement in OpenAPI, but the CLI should still send auth when configured.

## Image Inputs

The API expects images as `Base64Image` objects:

```json
{
  "type": "base64",
  "base64": "...",
  "format": "png"
}
```

CLI conventions:

- Flags ending in `--image`, `--reference-image`, `--style-image`, `--first-frame`, `--last-frame`, `--mask-image`, and similar should accept local file paths.
- The CLI should base64 encode file contents and infer `format` from extension, defaulting to `png`.
- Fields that include image dimensions should support `--image-size 128x128`, `--reference-image-size 128x128`, `--target-size 64x64`, and similar flags.
- Arrays of images should accept repeated flags, for example `--style-image a.png --style-image b.png`.
- Complex image arrays should also support JSON via `--style-images-json`, `--frames-json`, `--edit-images-json`, and `--body-json`.

## Async Jobs

Many endpoints return immediately with a background job id. The CLI should print job ids by default and optionally wait.

Async response status codes include `202` for most pro/background endpoints and `200` for some endpoints that still return queued job metadata.

Common async workflow:

```bash
pixellab post /generate-image-v2 --description "crystal sword" --image-size 128x128
pixellab get /background-jobs/{job_id} --job-id <uuid>
pixellab post /generate-image-v2 --description "crystal sword" --image-size 128x128 --wait --out ./out
```

The `--wait` implementation should:

- Poll `GET /background-jobs/{job_id}` when the response contains a job id.
- Stop when status is completed, failed, or timeout expires.
- Save returned images to `--out` when possible.
- Preserve raw response data in `--json` mode.

Common error statuses:

| Status | Meaning |
|---:|---|
| `400` | Invalid request state or validation outside schema. |
| `401` | Invalid API token. |
| `402` | Insufficient credits or generations. |
| `403` | Resource belongs to another user. |
| `404` | Resource or job not found. |
| `409` | Conflict, usually existing animation direction. |
| `422` | Schema validation error. |
| `423` | Resource still processing. |
| `429` | Too many requests or concurrent jobs. |
| `529` | Rate limit exceeded. |

## Shared Schemas And Enums

Common enum values:

| Type | Values |
|---|---|
| `CameraView` | `side`, `low top-down`, `high top-down` |
| `Direction` | `north`, `north-east`, `east`, `south-east`, `south`, `south-west`, `west`, `north-west` |
| `Outline` | `single color black outline`, `single color outline`, `selective outline`, `lineless` |
| `Shading` | `flat shading`, `basic shading`, `medium shading`, `detailed shading`, `highly detailed shading` |
| `Detail` | `low detail`, `medium detail`, `highly detailed` |
| `TilesetCameraView` | `low top-down`, `high top-down` |

Common object shapes:

| Schema | CLI representation |
|---|---|
| `ImageSize`, `OutputSize`, `TileSize`, `ProImageSize`, `V3OutputImageSize` | `WIDTHxHEIGHT`, for example `128x128`. |
| `Base64Image` | Local file path converted to base64 object. |
| `KeyframeImage` | `--start-image path --start-size WxH`, `--end-image path --end-size WxH`, or JSON. |
| `InpaintImage` | `--inpainting-image path --inpainting-size WxH`, or JSON. |
| `StyleOptions` | Individual booleans or JSON, default all true. |
| `TilesProStyleOptions` | Individual booleans or JSON, default all true. |
| `BoundingBox` | `--bounding-box X,Y,WIDTH,HEIGHT`. |
| `Point` | JSON for skeleton keypoints. |

## Endpoint Map

### Account

#### `GET /balance`

Command:

```bash
pixellab get /balance
```

Args: none.

Responses: `200`, `401`.

### Background Jobs

#### `GET /background-jobs/{job_id}`

Command:

```bash
pixellab get /background-jobs/{job_id} --job-id <uuid>
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--job-id` | yes | string | Path parameter. |

Responses: `200`, `401`, `404`, `422`, `429`.

### Documentation

#### `GET /llms.txt`

Command:

```bash
pixellab get /llms.txt
```

Args: none.

Responses: `200`.

### Create Image

#### `POST /generate-image-v2`

Generate image Pro. Async.

Command:

```bash
pixellab post /generate-image-v2 --description "..." --image-size 128x128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--image-size` | yes | size | Output image size. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `true`. |
| `--reference-image` | no | repeatable path | Up to 4 subject reference images. |
| `--style-image` | no | path | Style image. |
| `--style-options-json` | no | JSON | `color_palette`, `outline`, `detail`, `shading`. |

Responses: `202`, `401`, `402`, `422`, `429`.

#### `POST /generate-with-style-v2`

Generate with style Pro. Async.

Command:

```bash
pixellab post /generate-with-style-v2 --style-image a.png --description "..." --image-size 128x128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--style-image` | yes | repeatable path | 1-4 style images. |
| `--description` | yes | string | 1-2000 chars. |
| `--image-size` | yes | size | Output image size. |
| `--style-description` | no | string | Style hint. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `true`. |

Responses: `202`, `401`, `402`, `422`, `429`.

#### `POST /generate-ui-v2`

Generate UI Pro. Async.

Command:

```bash
pixellab post /generate-ui-v2 --description "medieval stone button" --image-size 128x64
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--image-size` | no | size | Output image size. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `true`. |
| `--concept-image` | no | path | Concept image. |
| `--color-palette` | no | string | Color palette text. |

Responses: `202`, `401`, `402`, `422`, `429`.

#### `POST /create-image-pixflux`

Command:

```bash
pixellab post /create-image-pixflux --description "cute dragon" --image-size 128x128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | Text prompt. |
| `--image-size` | yes | size | Output image size. |
| `--negative-description` | no | string | Deprecated. |
| `--text-guidance-scale` | no | number | Default `8`, range `1-20`. |
| `--outline` | no | enum | `Outline`. |
| `--shading` | no | enum | `Shading`. |
| `--detail` | no | enum | `Detail`. |
| `--view` | no | enum | `CameraView`. |
| `--direction` | no | enum | `Direction`. |
| `--isometric` | no | bool | Default `false`. |
| `--no-background` | no | bool | Default `false`. |
| `--background-removal-task` | no | enum | `remove_simple_background`, `remove_complex_background`. |
| `--init-image` | no | path | Initial image. |
| `--init-image-strength` | no | integer | Default `300`, range `1-999`. |
| `--color-image` | no | path | Palette image. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `401`, `402`, `422`, `429`, `529`.

#### `POST /create-image-pixen`

Command:

```bash
pixellab post /create-image-pixen --description "cute dragon" --image-size 128x128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | Text prompt. |
| `--image-size` | yes | size | Output image size. |
| `--outline` | no | enum | `Outline`. |
| `--detail` | no | enum | Default `highly detailed`. |
| `--view` | no | enum | `CameraView`. |
| `--direction` | no | enum | `Direction`. |
| `--no-background` | no | bool | Default `false`. |
| `--background-removal-task` | no | enum | `remove_simple_background`, `remove_complex_background`. |
| `--seed` | no | integer | Reproducible generation. |
| `--enhance-prompt` | no | bool | Default `false`. |

Responses: `200`, `401`, `402`, `422`, `429`, `529`.

#### `POST /create-image-bitforge`

Command:

```bash
pixellab post /create-image-bitforge --description "cute dragon" --image-size 128x128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | Text prompt. |
| `--image-size` | yes | size | Output image size. |
| `--negative-description` | no | string | Avoid prompt. |
| `--text-guidance-scale` | no | number | Default `8`, range `1-20`. |
| `--extra-guidance-scale` | no | number | Deprecated, default `3`. |
| `--style-strength` | no | number | Default `0`, range `0-100`. |
| `--outline` | no | enum | `Outline`. |
| `--shading` | no | enum | `Shading`. |
| `--detail` | no | enum | `Detail`. |
| `--view` | no | enum | `CameraView`. |
| `--direction` | no | enum | `Direction`. |
| `--isometric` | no | bool | Default `false`. |
| `--oblique-projection` | no | bool | Default `false`. |
| `--no-background` | no | bool | Default `false`. |
| `--coverage-percentage` | no | number | Canvas coverage. |
| `--init-image` | no | path | Initial image. |
| `--init-image-strength` | no | integer | Default `300`, range `1-999`. |
| `--style-image` | no | path | Style reference. |
| `--inpainting-image` | no | path | Inpaint source image. |
| `--mask-image` | no | path | White generates, black preserves. |
| `--color-image` | no | path | Palette image. |
| `--skeleton-guidance-scale` | no | number | Default `1`, range `0-5`. |
| `--skeleton-keypoints-json` | no | JSON | Skeleton points. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `401`, `402`, `422`, `429`, `529`.

### Image Operations

#### `POST /image-to-pixelart`

Command:

```bash
pixellab post /image-to-pixelart --image input.png --image-size 512x512 --output-size 128x128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--image` | yes | path | Image to convert. |
| `--image-size` | yes | size | Input image size. |
| `--output-size` | yes | size | Desired output size. |
| `--text-guidance-scale` | no | number | Default `8`. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `400`, `401`, `402`, `422`, `429`.

#### `POST /resize`

Command:

```bash
pixellab post /resize --description "wizard" --reference-image input.png --reference-image-size 64x64 --target-size 128x128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--reference-image` | yes | path | Image to resize. |
| `--reference-image-size` | yes | size | Original size. |
| `--target-size` | yes | size | Output size. |
| `--view` | no | enum | `CameraView`. |
| `--direction` | no | enum | `Direction`. |
| `--isometric` | no | bool | Default `false`. |
| `--oblique-projection` | no | bool | Default `false`. |
| `--no-background` | no | bool | Default `false`. |
| `--color-image` | no | path | Palette image. |
| `--init-image` | no | path | Initial image. |
| `--init-image-strength` | no | number | Default `150`. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `400`, `401`, `402`, `422`, `429`.

#### `POST /remove-background`

Command:

```bash
pixellab post /remove-background --image input.png --image-size 128x128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--image` | yes | path | PNG/JPEG image. |
| `--image-size` | yes | size | Input image size. |
| `--background-removal-task` | no | enum | `remove_simple_background`, `remove_complex_background`. |
| `--text` | no | string | Optional foreground description. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `401`, `402`, `422`, `429`.

### Animate

#### `POST /edit-animation-v2`

Edit animation Pro. Async.

Command:

```bash
pixellab post /edit-animation-v2 --description "add a glowing sword" --frames-json frames.json --image-size 64x64
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--frames-json` | yes | JSON | 2-16 frame image objects. |
| `--image-size` | yes | size | Output frame size. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `false`. |

Responses: `202`, `401`, `402`, `422`, `429`.

#### `POST /interpolation-v2`

Interpolate Pro. Async.

Command:

```bash
pixellab post /interpolation-v2 --start-image start.png --start-size 64x64 --end-image end.png --end-size 64x64 --action "sword slash" --image-size 64x64
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--start-image` | yes | path | Starting keyframe. |
| `--start-size` | yes | size | Starting keyframe size. |
| `--end-image` | yes | path | Ending keyframe. |
| `--end-size` | yes | size | Ending keyframe size. |
| `--action` | yes | string | 1-500 chars. |
| `--image-size` | yes | size | Output frame size. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `true`. |

Responses: `202`, `401`, `402`, `422`, `429`.

#### `POST /transfer-outfit-v2`

Transfer outfit Pro. Async.

Command:

```bash
pixellab post /transfer-outfit-v2 --reference-image outfit.png --reference-size 64x64 --frames-json frames.json --image-size 64x64
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--reference-image` | yes | path | Outfit or appearance reference. |
| `--reference-size` | yes | size | Reference size. |
| `--frames-json` | yes | JSON | 2-16 frame image objects. |
| `--image-size` | yes | size | Output frame size. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `false`. |
| `--additional-instructions` | no | string | Extra guidance. |

Responses: `202`, `401`, `402`, `422`, `429`.

#### `POST /animate-with-skeleton`

Command:

```bash
pixellab post /animate-with-skeleton --reference-image sprite.png --image-size 64x64 --skeleton-keypoints-json poses.json
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--image-size` | yes | size | Output image size. |
| `--reference-image` | yes | path | Reference image. |
| `--guidance-scale` | no | number | Default `4`, range `1-20`. |
| `--view` | no | enum | Default `side`. |
| `--direction` | no | enum | Default `east`. |
| `--isometric` | no | bool | Default `false`. |
| `--oblique-projection` | no | bool | Default `false`. |
| `--init-images-json` | no | JSON | Initial images. |
| `--init-image-strength` | no | integer | Default `300`, range `1-999`. |
| `--skeleton-keypoints-json` | no | JSON | Array of skeleton point arrays. |
| `--inpainting-images-json` | no | JSON | Optional inpainting images. |
| `--mask-images-json` | no | JSON | Optional masks. |
| `--color-image` | no | path | Palette image. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `401`, `402`, `422`, `429`, `529`.

#### `POST /animate-with-text`

Command:

```bash
pixellab post /animate-with-text --description "wizard" --action "walking" --reference-image sprite.png --image-size 64x64
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--image-size` | yes | size | Output image size. |
| `--description` | yes | string | Character description. |
| `--action` | yes | string | Action description. |
| `--reference-image` | yes | path | Reference image. |
| `--negative-description` | no | string | Avoid prompt. |
| `--text-guidance-scale` | no | number | Default `8`. |
| `--image-guidance-scale` | no | number | Default `1.4`. |
| `--n-frames` | no | integer | Default `4`; model always generates 4 frames. |
| `--start-frame-index` | no | integer | Default `0`. |
| `--view` | no | enum | Default `side`. |
| `--direction` | no | enum | Default `east`. |
| `--init-images-json` | no | JSON | Initial images. |
| `--init-image-strength` | no | integer | Default `300`, range `1-999`. |
| `--inpainting-images-json` | no | JSON | Existing animation frames. |
| `--mask-images-json` | no | JSON | Optional masks. |
| `--color-image` | no | path | Palette image. |
| `--seed` | no | integer | Default `0`. |

Responses: `200`, `401`, `402`, `422`, `429`, `529`.

#### `POST /animate-with-text-v2`

Animate with text Pro. Async.

Command:

```bash
pixellab post /animate-with-text-v2 --reference-image sprite.png --reference-image-size 64x64 --action "walking" --image-size 64x64
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--reference-image` | yes | path | Character/object reference. |
| `--reference-image-size` | yes | size | Reference size. |
| `--action` | yes | string | 1-500 chars. |
| `--image-size` | yes | size | Output frame size. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `true`. |
| `--view` | no | enum | `none`, `low top-down`, `high top-down`, `side`; default `none`. |
| `--direction` | no | enum | `none` or compass directions; default `none`. |

Responses: `202`, `401`, `402`, `422`, `429`.

#### `POST /animate-with-text-v3`

Command:

```bash
pixellab post /animate-with-text-v3 --first-frame idle.png --action "walking" --frame-count 8
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--first-frame` | yes | path | First frame, max 256x256. |
| `--last-frame` | no | path | Optional target frame, max 256x256. |
| `--action` | yes | string | 1-500 chars. |
| `--frame-count` | no | integer | Default `8`, range `4-16`, must be even. |
| `--seed` | no | integer | Default `0`. |
| `--no-background` | no | bool | Remove background. |
| `--enhance-prompt` | no | bool | Default `false`. |

Responses: `200`, `401`, `402`, `422`, `429`.

#### `POST /estimate-skeleton`

Command:

```bash
pixellab post /estimate-skeleton --image sprite.png
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--image` | no | path | Image to estimate skeleton from. |

Responses: `200`, `401`, `402`, `422`, `429`, `529`.

### Rotate

#### `POST /generate-8-rotations-v2`

Generate 8 rotations Pro. Async.

Command:

```bash
pixellab post /generate-8-rotations-v2 --image-size 128x128 --reference-image sprite.png
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--image-size` | yes | size | Output image size. |
| `--method` | no | enum | `rotate_character`, `create_with_style`, `create_from_concept`; default `rotate_character`. |
| `--reference-image` | no | path | Rotate source or style reference. |
| `--concept-image` | no | path | Concept image for `create_from_concept`. |
| `--description` | no | string | Character/item description. |
| `--style-description` | no | string | Style description. |
| `--view` | no | enum | `low top-down`, `high top-down`, `side`; default `low top-down`. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `true`. |

Responses: `202`, `401`, `402`, `422`, `429`.

#### `POST /generate-8-rotations-v3`

Command:

```bash
pixellab post /generate-8-rotations-v3 --first-frame south.png
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--first-frame` | yes | path | Reference frame, max 256x256. |
| `--no-background` | no | bool | Remove background. |
| `--seed` | no | integer | Default `0`. |

Responses: `200`, `401`, `402`, `422`, `429`.

#### `POST /rotate`

Command:

```bash
pixellab post /rotate --from-image sprite.png --image-size 128x128 --from-direction south --to-direction east
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--image-size` | yes | size | Output size. |
| `--from-image` | yes | path | Reference image to rotate. |
| `--image-guidance-scale` | no | number | Default `3`, range `1-20`. |
| `--view-change` | no | integer | Degrees to tilt. |
| `--direction-change` | no | integer | Degrees to rotate. |
| `--from-view` | no | enum | Default `side`. |
| `--to-view` | no | enum | Default `side`. |
| `--from-direction` | no | enum | Default `south`. |
| `--to-direction` | no | enum | Default `east`. |
| `--isometric` | no | bool | Default `false`. |
| `--oblique-projection` | no | bool | Default `false`. |
| `--init-image` | no | path | Initial image. |
| `--init-image-strength` | no | integer | Default `300`, range `1-999`. |
| `--mask-image` | no | path | Requires init image. |
| `--color-image` | no | path | Palette image. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `401`, `402`, `422`, `429`, `529`.

### Inpaint

#### `POST /inpaint-v3`

Inpaint image Pro. Async.

Command:

```bash
pixellab post /inpaint-v3 --description "golden crown" --inpainting-image image.png --inpainting-size 128x128 --mask-image mask.png --mask-size 128x128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--inpainting-image` | yes | path | 32x32 to 512x512. |
| `--inpainting-size` | yes | size | Source size. |
| `--mask-image` | yes | path | White generates, black preserves. |
| `--mask-size` | yes | size | Mask size. |
| `--context-image` | no | path | Deprecated. |
| `--context-size` | no | size | Context size. |
| `--bounding-box` | no | `x,y,w,h` | Deprecated. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `false`. |
| `--crop-to-mask` | no | bool | Default `true`. |

Responses: `202`, `401`, `402`, `422`, `429`.

#### `POST /inpaint`

Command:

```bash
pixellab post /inpaint --description "golden crown" --image-size 128x128 --inpainting-image image.png --mask-image mask.png
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | Text prompt. |
| `--image-size` | yes | size | Output size. |
| `--inpainting-image` | yes | path | Source image. |
| `--mask-image` | yes | path | White generates, black preserves. |
| `--negative-description` | no | string | Avoid prompt. |
| `--text-guidance-scale` | no | number | Default `3`, range `1-10`. |
| `--extra-guidance-scale` | no | number | Deprecated. |
| `--outline` | no | enum | `Outline`. |
| `--shading` | no | enum | `Shading`. |
| `--detail` | no | enum | `Detail`. |
| `--view` | no | enum | `CameraView`. |
| `--direction` | no | enum | `Direction`. |
| `--isometric` | no | bool | Default `false`. |
| `--oblique-projection` | no | bool | Default `false`. |
| `--no-background` | no | bool | Default `false`. |
| `--init-image` | no | path | Initial image. |
| `--init-image-strength` | no | integer | Default `300`. |
| `--color-image` | no | path | Palette image. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `401`, `402`, `422`, `429`, `529`.

### Edit

#### `POST /edit-images-v2`

Edit images Pro. Async.

Command:

```bash
pixellab post /edit-images-v2 --edit-images-json images.json --image-size 128x128 --description "make it icy"
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--edit-images-json` | yes | JSON | 1-16 images. |
| `--image-size` | yes | size | Output size. |
| `--method` | no | enum | `edit_with_text`, `edit_with_reference`; default `edit_with_text`. |
| `--description` | no | string | Required for text method. |
| `--reference-image` | no | path | Required for reference method. |
| `--reference-size` | no | size | Reference size. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `false`. |

Responses: `202`, `401`, `402`, `422`, `429`.

#### `POST /edit-image`

Edit image. Async.

Command:

```bash
pixellab post /edit-image --image input.png --image-size 128x128 --description "make it golden" --width 128 --height 128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--image` | yes | path | Reference image. |
| `--image-size` | yes | size | Reference size. |
| `--description` | yes | string | 1-500 chars. |
| `--width` | yes | integer | Target canvas width, 16-400. |
| `--height` | yes | integer | Target canvas height, 16-400. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `true`. |
| `--text-guidance-scale` | no | number | Default `8`. |
| `--color-image` | no | path | Color reference. |

Responses: `202`, `401`, `402`, `422`, `429`.

### Create Map

#### `POST /tilesets`

Create top-down Wang tileset asynchronously.

Command:

```bash
pixellab post /tilesets --lower-description "ocean" --upper-description "beach"
```

Args: same as `POST /create-tileset`.

Responses: `202`, `401`, `402`, `422`, `429`, `529`.

#### `GET /tilesets`

Command:

```bash
pixellab get /tilesets --limit 50 --offset 0
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--limit` | no | integer | Default `50`. |
| `--offset` | no | integer | Default `0`. |

Responses: `200`, `401`, `422`.

#### `POST /create-tileset`

Command:

```bash
pixellab post /create-tileset --lower-description "ocean" --upper-description "beach"
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--lower-description` | yes | string | Lower/base terrain. |
| `--upper-description` | yes | string | Upper/elevated terrain. |
| `--transition-description` | no | string | Transition area. |
| `--lower-base-tile-id` | no | string | Connected tileset lower base id. |
| `--upper-base-tile-id` | no | string | Connected tileset upper base id. |
| `--tile-size` | no | size | `16x16` or `32x32`, default `16x16`. |
| `--text-guidance-scale` | no | number | Default `8`, range `1-20`. |
| `--outline` | no | enum | `Outline`. |
| `--shading` | no | enum | `Shading`. |
| `--detail` | no | enum | `Detail`. |
| `--view` | no | enum | `low top-down`, `high top-down`; default `high top-down`. |
| `--tile-strength` | no | number | Default `1`, range `0.1-2`. |
| `--tileset-adherence-freedom` | no | number | Default `500`, range `0-900`. |
| `--tileset-adherence` | no | number | Default `100`, range `0-500`. |
| `--transition-size` | no | enum | `0`, `0.25`, `0.5`, `1`; default `0`. |
| `--lower-reference-image` | no | path | Lower terrain reference. |
| `--upper-reference-image` | no | path | Upper terrain reference. |
| `--transition-reference-image` | no | path | Transition reference. |
| `--color-image` | no | path | Palette reference. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `202`, `401`, `402`, `422`, `429`, `529`.

#### `GET /tilesets/{tileset_id}`

Command:

```bash
pixellab get /tilesets/{tileset_id} --tileset-id <uuid>
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--tileset-id` | yes | string | Path parameter. |

Responses: `200`, `401`, `404`, `422`, `423`.

#### `POST /tilesets-sidescroller`

Create sidescroller tileset asynchronously.

Command:

```bash
pixellab post /tilesets-sidescroller --lower-description "stone bricks" --transition-description "moss"
```

Args: same as `POST /create-tileset-sidescroller`.

Responses: `202`, `401`, `402`, `422`, `429`, `529`.

#### `POST /create-tileset-sidescroller`

Command:

```bash
pixellab post /create-tileset-sidescroller --lower-description "stone bricks" --transition-description "moss"
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--lower-description` | yes | string | Platform material. |
| `--transition-description` | no | string | Decorative top layer. |
| `--lower-base-tile-id` | no | string | Connected tileset base id. |
| `--tile-size` | no | size | `16x16` or `32x32`, default `16x16`. |
| `--text-guidance-scale` | no | number | Default `8`, range `1-20`. |
| `--outline` | no | enum | `Outline`. |
| `--shading` | no | enum | `Shading`. |
| `--detail` | no | enum | `Detail`. |
| `--tile-strength` | no | number | Default `1`, range `0.1-2`. |
| `--tileset-adherence-freedom` | no | number | Default `500`, range `0-900`. |
| `--tileset-adherence` | no | number | Default `100`, range `0-500`. |
| `--transition-size` | no | enum | `0`, `0.25`, `0.5`, `1`; default `0`. |
| `--lower-reference-image` | no | path | Platform reference. |
| `--transition-reference-image` | no | path | Transition reference. |
| `--color-image` | no | path | Palette reference. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `202`, `401`, `402`, `422`, `429`, `529`.

#### `POST /create-isometric-tile`

Command:

```bash
pixellab post /create-isometric-tile --description "grass on dirt" --image-size 32x32
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | Tile description. |
| `--image-size` | yes | size | Output image size. |
| `--text-guidance-scale` | no | number | Default `8`, range `1-20`. |
| `--outline` | no | string | Default `lineless`. |
| `--shading` | no | string | Default `basic shading`. |
| `--detail` | no | string | Default `medium detail`. |
| `--init-image` | no | path | Initial image. |
| `--init-image-strength` | no | integer | Default `300`, range `1-999`. |
| `--isometric-tile-size` | no | integer | Default `16`. |
| `--isometric-tile-shape` | no | enum | `thick tile`, `thin tile`, `block`; default `block`. |
| `--color-image` | no | path | Palette image. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `202`, `401`, `402`, `422`, `429`, `529`.

#### `GET /isometric-tiles/{tile_id}`

Command:

```bash
pixellab get /isometric-tiles/{tile_id} --tile-id <uuid>
```

Args: `--tile-id` required.

Responses: `200`, `401`, `404`, `422`, `423`.

#### `GET /isometric-tiles`

Command:

```bash
pixellab get /isometric-tiles --limit 50 --offset 0
```

Args: `--limit`, `--offset`.

Responses: `200`, `401`, `422`.

#### `POST /create-tiles-pro`

Command:

```bash
pixellab post /create-tiles-pro --description "1). grass 2). stone" --tile-type isometric --tile-size 32
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | Numbered tile descriptions recommended. |
| `--tile-type` | no | enum | `hex`, `hex_pointy`, `isometric`, `octagon`, `square_topdown`; default `isometric`. |
| `--tile-size` | no | integer | Default `32`, range `16-256`. |
| `--tile-height` | no | integer | Non-square tile height. |
| `--tile-view` | no | enum | `top-down`, `high top-down`, `low top-down`, `side`; default `low top-down`. |
| `--tile-view-angle` | no | number | `0-90`, overrides `tile_view`. |
| `--tile-depth-ratio` | no | number | `0.0-1.0`. |
| `--seed` | no | integer | Reproducible generation. |
| `--style-images-json` | no | JSON | Style references. |
| `--style-options-json` | no | JSON | Style copy options. |

Responses: `202`, `401`, `402`, `422`, `429`.

#### `GET /tiles-pro/{tile_id}`

Command:

```bash
pixellab get /tiles-pro/{tile_id} --tile-id <uuid>
```

Args: `--tile-id` required.

Responses: `200`, `401`, `404`, `422`, `423`.

### Map Objects

#### `POST /map-objects`

Command:

```bash
pixellab post /map-objects --description "wooden barrel" --image-size 128x128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--image-size` | no | size | Default `128x128`. |
| `--view` | no | enum | `low top-down`, `high top-down`, `side`; default `high top-down`. |
| `--outline` | no | string | Default `single color outline`. |
| `--shading` | no | string | Default `medium shading`. |
| `--detail` | no | string | Default `medium detail`. |
| `--text-guidance-scale` | no | number | Default `8`, range `1-20`. |
| `--init-image` | no | path | Initial image. |
| `--init-image-strength` | no | integer | Default `300`, range `1-999`. |
| `--color-image` | no | path | Palette image. |
| `--background-image` | no | path | Style matching or inpainting background. |
| `--inpainting-json` | no | JSON | Mask, oval, or rectangle inpainting config. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `401`, `402`, `422`, `429`.

### Character From Template

#### `POST /create-character-with-4-directions`

Command:

```bash
pixellab post /create-character-with-4-directions --description "blue wizard" --image-size 64x64
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--image-size` | yes | size | Rotation frame size. |
| `--async-mode` | no | bool | Default `true`. |
| `--text-guidance-scale` | no | number | Default `8`. |
| `--outline` | no | string | Default `single color black outline`. |
| `--shading` | no | string | Default `basic shading`. |
| `--detail` | no | string | Default `medium detail`. |
| `--view` | no | string | Default `low top-down`. |
| `--isometric` | no | bool | Default `false`. |
| `--color-image` | no | path | Palette image. |
| `--force-colors` | no | bool | Default `false`. |
| `--proportions-json` | no | JSON | Humanoid proportions. |
| `--template-id` | no | string | `mannequin`, `bear`, `cat`, `dog`, `horse`, `lion`. |
| `--seed` | no | integer | Reproducible generation. |
| `--directions-json` | no | JSON | Optional reference images by direction. |
| `--output-type` | no | string | Default `dict`. |

Responses: `200`, `401`, `402`, `422`, `429`.

#### `POST /create-character-with-8-directions`

Command:

```bash
pixellab post /create-character-with-8-directions --description "blue wizard" --image-size 64x64
```

Args: same as 4-direction character plus `--mode` default `standard`.

Responses: `200`, `401`, `402`, `422`, `429`.

#### `POST /create-character-pro`

Command:

```bash
pixellab post /create-character-pro --description "blue wizard" --image-size 96x96
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--image-size` | yes | size | Pro frame size, 32-168. |
| `--method` | no | enum | `create_with_style`, `create_from_concept`, `rotate_character`; default `create_with_style`. |
| `--view` | no | enum | `low top-down`, `high top-down`, `side`; default `low top-down`. |
| `--template-id` | no | string | Default `mannequin`. |
| `--concept-image` | no | path | Max 1024x1024. |
| `--reference-image` | no | path | Max 168x168. |
| `--style-description` | no | string | Style hint. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `true`. |

Responses: `200`, `401`, `402`, `422`, `429`.

#### `POST /create-character-v3`

Command:

```bash
pixellab post /create-character-v3 --description "blue wizard" --image-size 64x64 --enhance-prompt
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--reference-image` | no | path | South-facing reference image. |
| `--image-size` | no | size | Advisory or from-scratch size, 32-256. |
| `--view` | no | enum | `low top-down`, `high top-down`, `side`; default `low top-down`. |
| `--template-id` | no | string | Default `mannequin`. |
| `--name` | no | string | Display name. |
| `--seed` | no | integer | Reproducible generation. |
| `--no-background` | no | bool | Default `true`. |
| `--outline` | no | string | Ignored with reference image. |
| `--detail` | no | string | Ignored with reference image. |
| `--enhance-prompt` | no | bool | Default `false`. |

Responses: `200`, `401`, `402`, `422`, `429`.

#### `POST /characters/animations`

Command:

```bash
pixellab post /characters/animations --character-id <uuid> --action-description "walking"
```

Args: same as `POST /animate-character`.

Responses: `200`, `422`.

#### `POST /animate-character`

Command:

```bash
pixellab post /animate-character --character-id <uuid> --mode v3 --action-description "walking" --frame-count 8
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--character-id` | yes | string | Existing character. |
| `--animation-name` | no | string | Display name. |
| `--description` | no | string | Override character description. |
| `--action-description` | no | string | Required for custom mode. |
| `--async-mode` | no | bool | Default `true`. |
| `--mode` | no | string | `template`, `v3`, or pro/custom modes as supported. |
| `--template-animation-id` | no | string | Required for template mode. |
| `--frame-count` | no | integer | Default `8`, v3 only, 4-16 even. |
| `--text-guidance-scale` | no | number | Default `8`, template mode only. |
| `--outline` | no | string | Template mode only. |
| `--shading` | no | string | Template mode only. |
| `--detail` | no | string | Template mode only. |
| `--directions` | no | CSV | Directions to animate. |
| `--isometric` | no | bool | Default `false`. |
| `--color-image` | no | path | Palette image. |
| `--force-colors` | no | bool | Default `false`. |
| `--seed` | no | integer | Reproducible generation. |
| `--enhance-prompt` | no | bool | Default `false`. |

Responses: `200`, `401`, `402`, `404`, `422`, `429`.

### Characters

#### `POST /create-character-state`

Command:

```bash
pixellab post /create-character-state --character-id <uuid> --edit-description "wearing red armor"
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--character-id` | yes | string | Source character id. |
| `--edit-description` | yes | string | 1-1000 chars. |
| `--no-background` | no | bool | Default `true`. |
| `--seed` | no | integer | Reproducible generation. |
| `--use-color-palette-from-reference` | no | bool | Default `false`. |

Responses: `200`, `400`, `401`, `402`, `404`, `422`, `429`.

### Character Management

#### `GET /characters`

Command:

```bash
pixellab get /characters --limit 50 --offset 0
```

Args: `--limit`, `--offset`.

Responses: `200`, `401`, `422`, `429`.

#### `GET /characters/{character_id}`

Command:

```bash
pixellab get /characters/{character_id} --character-id <uuid>
```

Args: `--character-id` required.

Responses: `200`, `401`, `403`, `404`, `422`, `429`.

#### `DELETE /characters/{character_id}`

Command:

```bash
pixellab delete /characters/{character_id} --character-id <uuid>
```

Args: `--character-id` required.

Responses: `200`, `422`.

#### `GET /characters/{character_id}/zip`

Command:

```bash
pixellab get /characters/{character_id}/zip --character-id <uuid> --out character.zip
```

Args: `--character-id` required, `--out` recommended.

Responses: `200`, `404`, `422`, `423`.

#### `PATCH /characters/{character_id}/tags`

Command:

```bash
pixellab patch /characters/{character_id}/tags --character-id <uuid> --tag wizard --tag fire
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--character-id` | yes | string | Path parameter. |
| `--tag` | yes | repeatable string | Up to 20 tags. |

Responses: `200`, `400`, `401`, `403`, `404`, `422`, `429`.

### Objects

#### `POST /create-1-direction-object`

Command:

```bash
pixellab post /create-1-direction-object --description "wooden barrel" --size 128 --view sidescroller
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--size` | no | integer | Square size 32-256, default 64. Mutually exclusive with style images. |
| `--view` | no | enum | `top-down`, `sidescroller`; default `top-down`. |
| `--style-image` | no | repeatable path | PNG/JPEG max 256x256. |
| `--item-description` | no | repeatable string | Per-object descriptions for review packs. |

Responses: `200`, `401`, `402`, `422`, `429`.

#### `POST /create-8-direction-object`

Command:

```bash
pixellab post /create-8-direction-object --description "stone fountain" --size 128 --view "low top-down"
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--size` | no | integer | Square size 32-256, default 64. |
| `--view` | no | enum | `low top-down`, `high top-down`, `side`; default `low top-down`. |
| `--reference-image` | no | path | Rotates exact image. Mutually exclusive with style image and size. |
| `--style-image` | no | path | Style reference. Mutually exclusive with reference image and size. |

Responses: `200`, `401`, `402`, `422`, `429`.

#### `POST /objects/{object_id}/animations`

Command:

```bash
pixellab post /objects/{object_id}/animations --object-id <uuid> --animation-description "walking" --display-name walk --frame-count 8
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--object-id` | yes | string | Path parameter. |
| `--mode` | no | enum | `pro`, `v3`; default `v3`. |
| `--animation-description` | no | string | Required for new animations. |
| `--directions` | no | CSV | Do not pass for 1-direction objects. |
| `--animation-group-id` | no | string | Extend existing animation group. |
| `--display-name` | no | string | UI/export name. |
| `--frame-count` | no | integer | v3 even 4-16; pro depends on canvas. |
| `--replace-existing` | no | bool | Default `false`. |
| `--custom-start-frame` | no | path | v3 only. |
| `--end-frame` | no | path | v3 interpolation target. |
| `--enhance-prompt` | no | bool | Default `false`. |

Responses: `200`, `400`, `401`, `402`, `404`, `409`, `422`, `429`.

#### `POST /objects/{object_id}/states`

Command:

```bash
pixellab post /objects/{object_id}/states --object-id <uuid> --edit-description "make it golden"
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--object-id` | yes | string | Path parameter. |
| `--edit-description` | yes | string | 1-1000 chars. |
| `--seed` | no | integer | Reproducible generation. |

Responses: `200`, `400`, `401`, `402`, `404`, `422`, `429`.

#### `POST /objects/{object_id}/select-frames`

Command:

```bash
pixellab post /objects/{object_id}/select-frames --object-id <uuid> --index 0 --index 3 --common-tag props
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--object-id` | yes | string | Path parameter. |
| `--index` | yes | repeatable integer | 0-based frame index. |
| `--common-tag` | no | string | Applied to created objects. |

Responses: `200`, `400`, `401`, `404`, `422`.

#### `POST /objects/{object_id}/dismiss-review`

Command:

```bash
pixellab post /objects/{object_id}/dismiss-review --object-id <uuid>
```

Args: `--object-id` required.

Responses: `200`, `400`, `401`, `404`, `422`.

### Object Management

#### `GET /objects`

Command:

```bash
pixellab get /objects --limit 50 --offset 0
```

Args: `--limit`, `--offset`.

Responses: `200`, `401`, `422`.

#### `GET /objects/{object_id}`

Command:

```bash
pixellab get /objects/{object_id} --object-id <uuid>
```

Args: `--object-id` required.

Responses: `200`, `401`, `403`, `404`, `422`.

#### `DELETE /objects/{object_id}`

Command:

```bash
pixellab delete /objects/{object_id} --object-id <uuid>
```

Args: `--object-id` required.

Responses: `200`, `401`, `403`, `404`, `422`.

#### `PATCH /objects/{object_id}/tags`

Command:

```bash
pixellab patch /objects/{object_id}/tags --object-id <uuid> --tag barrel --tag prop
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--object-id` | yes | string | Path parameter. |
| `--tag` | yes | repeatable string | Up to 20 tags. |

Responses: `200`, `400`, `401`, `403`, `404`, `422`.

### Enhance Prompt

#### `POST /enhance-pixen-prompt`

Command:

```bash
pixellab post /enhance-pixen-prompt --description "dragon" --image-size 128x128
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--image-size` | yes | size | Target size. |
| `--outline` | no | enum | `Outline`. |
| `--detail` | no | enum | Default `highly detailed`. |
| `--view` | no | enum | `CameraView`. |
| `--direction` | no | enum | `Direction`. |
| `--no-background` | no | bool | Default `false`. |

Responses: `200`, `401`, `402`, `422`.

#### `POST /enhance-character-v3-prompt`

Command:

```bash
pixellab post /enhance-character-v3-prompt --description "blue wizard" --image-size 64x64
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--description` | yes | string | 1-2000 chars. |
| `--image-size` | yes | size | Target size. |
| `--view` | no | enum | `low top-down`, `high top-down`, `side`; default `low top-down`. |
| `--outline` | no | string | Outline hint. |
| `--detail` | no | string | Detail hint. |

Responses: `200`, `401`, `402`, `422`.

#### `POST /enhance-animation-v3-prompt`

Command:

```bash
pixellab post /enhance-animation-v3-prompt --first-frame idle.png --action "walking"
```

Args:

| Arg | Required | Type | Notes |
|---|---:|---|---|
| `--first-frame` | yes | path | First frame image. |
| `--last-frame` | no | path | Optional end frame. |
| `--action` | yes | string | 1-500 chars. |

Responses: `200`, `401`, `402`, `422`.

## Animation Workflow Notes

For readable combat animations, prefer controlled keyframes over pure text-only generation when possible. The v3 animation docs classify `4` frames as suitable for simple loops or idles, `8` frames as standard movement, and `16` frames as the better fit for complex actions such as attack combos.

Recommended combat workflow:

1. Start from a clean idle or ready frame that already contains the weapon and silhouette you want preserved.
2. Create or choose an end-pose frame with the weapon fully extended or the strike clearly readable.
3. Use v3 interpolation through `POST /objects/{object_id}/animations` with `--custom-start-frame` and `--end-frame`, or use `POST /animate-with-text-v3` with `--first-frame` and `--last-frame`.
4. Use `--frame-count 8` for short attacks and `--frame-count 16` for heavy attacks, combos, or wind-up plus recovery.
5. Keep the prompt action-focused: describe wind-up, strike, follow-through, and recovery, and explicitly forbid walking, jumping, camera movement, new props, and background changes when those are unwanted.

Object animation example:

```bash
pixellab post /objects/{object_id}/animations \
  --object-id <uuid> \
  --animation-description "aggressive sword attack: deep wind-up, heavy forward slash, strong follow-through, recover to idle; feet stay planted; big readable weapon motion; no walking, no jumping, no camera movement, no new objects, no background" \
  --display-name sword_attack \
  --frame-count 16 \
  --custom-start-frame idle.png \
  --end-frame slash_pose.png \
  --wait
```

Standalone v3 example:

```bash
pixellab post /animate-with-text-v3 \
  --first-frame idle.png \
  --last-frame slash_pose.png \
  --action "heavy sword slash with planted feet, clear wind-up, strike, follow-through, and recovery" \
  --frame-count 16 \
  --wait
```

Avoid relying on `--enhance-prompt` for precise combat intent until the enhanced prompt is inspected. It can soften a hard attack into subtle motion, which may make the result read as an idle or gesture instead of combat.

## Go Implementation Notes

- Use `net/http` and `encoding/json` initially; no generated client is required for phase one.
- Keep a schema map for endpoint metadata so every command can be routed through a single generic request builder.
- Normalize flag names by converting JSON snake_case to CLI kebab-case.
- Preserve `--body-json` so new API fields can be used before typed flags are added.
- Decode path placeholders from flags before request execution.
- Encode repeated flags into arrays.
- Encode size flags into `{ "width": W, "height": H }` objects.
- Convert file flags into `Base64Image` objects.
- Save binary/ZIP responses directly when content type is not JSON.
- For JSON image responses, support `--out` by walking known response image fields and writing decoded base64 data when present.
- For async responses, detect `background_job_id`, `job_id`, `id`, or equivalent job fields conservatively, then use `GET /background-jobs/{job_id}` for `--wait`.
- Return non-zero exit codes for non-2xx responses, failed jobs, file decoding failures, and timeout.

## Implementation Priority

Support every endpoint from the start through the generic method/path dispatcher. Typed convenience parsing should still cover the documented flags above, with `--body-json` as the compatibility fallback for complex or uncommon request shapes.
