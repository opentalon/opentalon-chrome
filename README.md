# opentalon-chrome

[![CI](https://github.com/opentalon/opentalon-chrome/actions/workflows/ci.yml/badge.svg)](https://github.com/opentalon/opentalon-chrome/actions/workflows/ci.yml)

A headless Chrome plugin for [OpenTalon](https://github.com/opentalon/opentalon). It connects to a Chrome sidecar running with the Chrome DevTools Protocol (CDP) and exposes browser actions as LLM tools: navigate pages, extract text/HTML, take screenshots, click elements, type text, and evaluate JavaScript.

It also supports an **interactive login** workflow: a visible Chrome browser (with noVNC) is deployed alongside the agent, the user logs in to a service manually, and the resulting session cookies are captured via CDP, stored per-entity in SQLite, and replayed in headless Chrome for future automated tasks.

## Architecture

```
OpenTalon ──gRPC──► opentalon-chrome ──CDP──► Chrome headless (sidecar)
                                    ╰──CDP──► Chrome + noVNC (login sidecar, optional)
```

The plugin binary (`opentalon-chrome`) runs as a subprocess of OpenTalon (or in TCP mode as a Docker sidecar). It speaks the OpenTalon plugin protocol over a Unix socket (or TCP port) and forwards each action to a Chrome instance via the Chrome DevTools Protocol HTTP/WebSocket API.

When the optional login sidecar is configured, a second Chrome instance (with a noVNC web UI) is available for interactive logins. The captured cookies are persisted in a local SQLite database and can be replayed in the headless instance.

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

| config.yaml key    | Env var override          | Default                 | Description                                                               |
|--------------------|---------------------------|-------------------------|---------------------------------------------------------------------------|
| `cdp_url`          | `CHROME_CDP_URL`          | `http://localhost:9222` | Chrome DevTools Protocol base URL (headless Chrome)                       |
| `screenshot_dir`   | `CHROME_SCREENSHOT_DIR`   | `os.TempDir()`          | Directory where screenshot PNG files are saved                            |
| `timeout`          | `CHROME_TIMEOUT`          | `30s`                   | Per-action deadline as a Go duration string (e.g. `45s`)                  |
| —                  | `CHROME_GRPC_PORT`        | —                       | If set, listen on this TCP port instead of a Unix socket                  |
| `data_dir`         | `CHROME_DATA_DIR`         | `os.TempDir()`          | Directory for the credential SQLite database (`state.db`)                 |
| `login_cdp_url`    | `CHROME_LOGIN_CDP_URL`    | —                       | CDP URL of the interactive login Chrome instance (noVNC sidecar)          |
| `login_url`        | `CHROME_LOGIN_URL`        | —                       | noVNC web UI URL to present to the user for manual login                  |
| `login_password`   | `CHROME_LOGIN_PASSWORD`   | —                       | noVNC session password                                                    |

## Available actions

### Headless browser actions

All headless actions open a fresh browser tab, perform the operation, and close the tab. State is not persisted between calls.

| Action        | Required args                   | Optional args | Returns                                                               |
|---------------|---------------------------------|---------------|-----------------------------------------------------------------------|
| `navigate`    | `url`                           | —             | Confirmation with page title                                          |
| `get_text`    | `url`                           | `selector`    | Visible text content (default: `body`)                                |
| `get_html`    | `url`                           | `selector`    | Outer HTML (default: `:root`)                                         |
| `screenshot`  | `url`                           | `selector`    | File path + inline `data:image/png;base64,…` (if ≤ 40 KB)            |
| `click`       | `url`, `selector`               | —             | Confirmation message                                                  |
| `type_text`   | `url`, `selector`, `text`       | —             | Confirmation message                                                  |
| `evaluate`    | `url`, `script`                 | —             | JSON-encoded JavaScript result                                        |
| `navigate_with_cookies` | `url`, `cookies`      | —             | Page title after navigating with the supplied session cookies         |

### Interactive login actions

These actions require the login sidecar to be configured (see [Browser login sessions](#browser-login-sessions)).

| Action               | Required args      | Optional args | Returns                                                        |
|----------------------|--------------------|---------------|----------------------------------------------------------------|
| `start_login_session`| —                  | —             | noVNC URL and session password for the user to open            |
| `get_cookies`        | `url`              | `domain`      | JSON array of cookies from the interactive Chrome instance     |
| `save_credentials`   | `name`, `cookies`  | —             | Confirmation; stores cookies under this entity + name          |
| `get_credentials`    | `name`             | —             | Cookie JSON for the given credential name                      |
| `list_credentials`   | —                  | —             | List of saved credential names for this entity                 |
| `delete_credentials` | `name`             | —             | Confirmation of deletion                                       |

Credentials are isolated per entity (user/group). The same `name` can be used by different entities without conflict.

### Screenshot inline delivery

The `screenshot` action always saves the PNG to `screenshot_dir` and returns the path. When the PNG is ≤ 40 KB, it also embeds a `data:image/png;base64,…` data URL in the response so the image can be forwarded directly to the user (e.g. as a Slack file upload by the channel plugin). Larger screenshots return a note indicating the file size — access the file at the returned path.

## Browser login sessions

Some services require an interactive login that cannot be automated (e.g. CAPTCHA, MFA, SSO redirects). The login session workflow handles this by deploying a visible Chrome instance alongside the agent. The user logs in through their browser, and OpenTalon captures the resulting session cookies for future automated use.

### How it works

```
1. Agent calls start_login_session
   → returns noVNC URL + session password

2. User opens the URL in their browser
   → sees a full Chrome browser via noVNC
   → navigates to https://app.example.com, logs in manually

3. User tells the agent: "I'm done"

4. Agent calls get_cookies(url=https://app.example.com, domain=app.example.com)
   → returns JSON array of session cookies

5. Agent calls save_credentials(name=example-work, cookies=<json>)
   → stored in SQLite keyed by (entity_id, "example-work")

6. Future automated task:
   Agent calls get_credentials(name=example-work)
   → retrieves stored cookies
   Agent calls navigate_with_cookies(url=https://app.example.com/dashboard, cookies=<json>)
   → opens page in headless Chrome with cookies pre-set → returns page title confirming login
```

Multiple accounts per service are supported by using different names:

```
save_credentials(name=example-personal, cookies=...)
save_credentials(name=example-work, cookies=...)
```

### Kubernetes setup

When using the [k8s-operator](https://github.com/opentalon/k8s-operator), add `spec.chromeLogin` to your `OpenTalonInstance`:

```yaml
apiVersion: opentalon.io/v1alpha1
kind: OpenTalonInstance
metadata:
  name: my-agent
spec:
  chromeLogin:
    image: lscr.io/linuxserver/chromium:latest
    vncPort: 3000      # noVNC web UI port (default: 3000)
    cdpPort: 9222      # CDP port inside the sidecar (default: 9222, exposed as 9223 on the Service)
    ingress:
      enabled: true
      className: nginx
      host: chrome-login.example.com
      tlsSecretName: chrome-login-tls           # cert-manager writes the certificate here
      annotations:
        cert-manager.io/cluster-issuer: letsencrypt-prod
        nginx.ingress.kubernetes.io/ssl-redirect: "true"
```

The operator will:
- Deploy a `lscr.io/linuxserver/chromium` sidecar container in the agent pod
- Create a Kubernetes `Secret` with a randomly generated noVNC session password (never regenerated after the first creation)
- Create a `ClusterIP` Service exposing the noVNC web UI and the CDP endpoint
- Create an `Ingress` resource (when `ingress.enabled: true`) with optional TLS via cert-manager
- Inject `CHROME_LOGIN_CDP_URL`, `CHROME_LOGIN_URL`, `CHROME_LOGIN_PASSWORD`, and `CHROME_DATA_DIR` into the main container automatically

Once deployed, `status.chromeLoginURL` will contain the URL to share with the user.

### Docker Compose setup

To test the login workflow locally with Docker Compose, add a `chromium` service and wire the environment variables:

```yaml
services:
  opentalon:
    image: ghcr.io/opentalon/opentalon:latest
    depends_on: [opentalon-chrome, chrome, chrome-login]

  opentalon-chrome:
    build: .
    environment:
      - CHROME_CDP_URL=http://chrome:9222
      - CHROME_LOGIN_CDP_URL=http://chrome-login:9222
      - CHROME_LOGIN_URL=http://localhost:3000        # presented to the user
      - CHROME_LOGIN_PASSWORD=changeme
      - CHROME_DATA_DIR=/data/chrome-credentials
      - CHROME_SCREENSHOT_DIR=/data/screenshots
    volumes:
      - chrome_data:/data

  chrome:
    image: chromedp/headless-shell:latest
    command: [--no-sandbox, --remote-debugging-address=0.0.0.0, --remote-debugging-port=9222]

  chrome-login:
    image: lscr.io/linuxserver/chromium:latest
    environment:
      - CUSTOM_USER=opentalon
      - PASSWORD=changeme
      - CHROME_CLI=--remote-debugging-port=9222 --remote-debugging-address=0.0.0.0 --no-sandbox
    ports:
      - "3000:3000"   # noVNC web UI

volumes:
  chrome_data:
```

In `config.yaml`:

```yaml
plugins:
  chrome:
    enabled: true
    plugin: "grpc://opentalon-chrome:50051"
    config:
      cdp_url: "http://chrome:9222"
      login_cdp_url: "http://chrome-login:9222"
      login_url: "http://localhost:3000"
      login_password: "changeme"
      data_dir: "/data/chrome-credentials"
      screenshot_dir: "/data/screenshots"
      timeout: "45s"
```

## Development

```sh
# Build
make build

# Run tests
make test

# Lint
make lint
```
