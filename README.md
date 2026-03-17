# opentalon-chrome

[![CI](https://github.com/opentalon/opentalon-chrome/actions/workflows/ci.yml/badge.svg)](https://github.com/opentalon/opentalon-chrome/actions/workflows/ci.yml)

A headless Chrome plugin for [OpenTalon](https://github.com/opentalon/opentalon). It connects to a Chrome sidecar running with the Chrome DevTools Protocol (CDP) and exposes browser actions as LLM tools: navigate pages, extract text/HTML, take screenshots, click elements, type text, and evaluate JavaScript.

## Architecture

```
OpenTalon ──gRPC──► opentalon-chrome ──CDP──► Chrome headless (sidecar)
```

The plugin binary (`opentalon-chrome`) runs as a subprocess of OpenTalon (or in TCP mode as a Docker sidecar). It speaks the OpenTalon plugin protocol over a Unix socket (or TCP port) and forwards each action to a Chrome instance via the Chrome DevTools Protocol HTTP/WebSocket API.

## Sidecar setup

Run Chrome headless alongside OpenTalon. The plugin connects to it via `CHROME_CDP_URL`.

### Docker Compose example

```yaml
services:
  opentalon:
    image: ghcr.io/opentalon/opentalon:latest
    environment:
      - CHROME_GRPC_PORT=50052
    depends_on:
      - chrome
      - opentalon-chrome

  opentalon-chrome:
    build: .  # or use a prebuilt image
    environment:
      - CHROME_GRPC_PORT=50051
      - CHROME_CDP_URL=http://chrome:9222
      - CHROME_SCREENSHOT_DIR=/data/screenshots
    volumes:
      - screenshots:/data/screenshots

  chrome:
    image: chromedp/headless-shell:latest
    # OR: gcr.io/zenika-hub/alpine-chrome
    command:
      - --no-sandbox
      - --remote-debugging-address=0.0.0.0
      - --remote-debugging-port=9222

volumes:
  screenshots:
```

Then in your `config.yaml`:

```yaml
plugins:
  chrome:
    enabled: true
    plugin: "grpc://opentalon-chrome:50051"
    config:
      cdp_url: "http://chrome:9222"
      screenshot_dir: "/data/screenshots"
      timeout: "45s"
```

### Local binary example

```yaml
plugins:
  chrome:
    enabled: true
    plugin: "./plugins/opentalon-chrome/opentalon-chrome"
    config:
      cdp_url: "http://localhost:9222"
      screenshot_dir: "/tmp"
      timeout: "30s"
```

Start Chrome headless locally:

```sh
google-chrome --headless --no-sandbox --remote-debugging-port=9222
# or
chromium --headless --no-sandbox --remote-debugging-port=9222
```

## Configuration

Configuration is delivered as a JSON string by the OpenTalon host via the `Configure` RPC call during the Capabilities handshake (populated from the plugin's `config:` block in `config.yaml`). Individual `CHROME_*` environment variables override those values and can be used for ad-hoc overrides or standalone testing.

| config.yaml key   | Env var override         | Default              | Description                                                  |
|-------------------|--------------------------|----------------------|--------------------------------------------------------------|
| `cdp_url`         | `CHROME_CDP_URL`         | `http://localhost:9222` | Chrome DevTools Protocol base URL                         |
| `screenshot_dir`  | `CHROME_SCREENSHOT_DIR`  | `os.TempDir()`       | Directory where screenshot PNG files are saved               |
| `timeout`         | `CHROME_TIMEOUT`         | `30s`                | Per-action deadline as a Go duration string (e.g. `45s`)     |
| —                 | `CHROME_GRPC_PORT`       | —                    | If set, listen on this TCP port instead of a Unix socket     |

## Available actions

| Action      | Required args     | Optional args   | Returns                                 |
|-------------|-------------------|-----------------|-----------------------------------------|
| `navigate`  | `url`             | —               | Confirmation with page title            |
| `get_text`  | `url`             | `selector`      | Visible text content (default: `body`)  |
| `get_html`  | `url`             | `selector`      | Outer HTML (default: `:root`)           |
| `screenshot`| `url`             | `selector`      | File path + inline `data:image/png;base64,…` (if ≤ 40 KB) |
| `click`     | `url`, `selector` | —               | Confirmation message                    |
| `type_text` | `url`, `selector`, `text` | —     | Confirmation message                    |
| `evaluate`  | `url`, `script`   | —               | JSON-encoded JavaScript result          |

All actions open a fresh browser tab, perform the operation, and close the tab. State is not persisted between calls.

### Screenshot inline delivery

The `screenshot` action always saves the PNG to `screenshot_dir` and returns the path. When the PNG is ≤ 40 KB, it also embeds a `data:image/png;base64,…` data URL in the response so the image can be forwarded directly to the user (e.g. as a Slack file upload by the channel plugin). Larger screenshots return a note indicating the file size — access the file at the returned path.

## Development

```sh
# Build
make build

# Run tests
make test

# Lint
make lint
```

### Local dependency replace (during opentalon development)

```sh
go mod edit -replace github.com/opentalon/opentalon=../opentalon
go mod tidy
```
