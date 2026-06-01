# pixellab-cli

Generic CLI for the PixelLab v2 API.

```bash
pixellab get /balance
pixellab post /create-image-pixen --description "cute dragon" --image-size 128x128 --out ./out
pixellab post /generate-image-v2 --description "crystal sword" --image-size 128x128 --wait --out ./out
pixellab post /objects/{object_id}/animations --object-id <uuid> --animation-description "heavy sword slash" --frame-count 16 --wait
```

Set `PIXELLAB_API_KEY` in your environment or in a `.env` file in the directory where you run the CLI. You can also pass `--token` explicitly.

See `docs/api-v2-cli-map.md` for the full endpoint map, flag contract, and animation workflow notes.
