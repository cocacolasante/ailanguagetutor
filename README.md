# LinguaAI — AI-Powered Language Tutor

A real-time conversational language tutor powered by an LLM backend, ElevenLabs text-to-speech, Web Speech API voice input, and Stripe subscription billing.

---

## Features

- **Live conversation practice** — Streamed AI responses via Server-Sent Events (SSE)
- **10 languages** — Italian, Spanish, Portuguese, French, German, Japanese, Russian, Romanian, Chinese, and more
- **5 proficiency levels** — Beginner through Fluent, each with distinct teaching styles
- **20 curated topics** — Organized across Everyday Life, Social, Travel, Health, and Professional categories
- **Voice I/O** — ElevenLabs TTS playback + Web Speech API voice input
- **Translation assist** — Inline translation of any AI message
- **Conversation memory** — Rolling context across sessions per user/language/level
- **Stripe billing** — 7-day free trial or immediate subscription; Customer Portal for self-service
- **Email verification** — New users verify their address before accessing the platform
- **Admin panel** — Manage subscriptions, grant/revoke access, view email verification status

---

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.21, [chi](https://github.com/go-chi/chi) router |
| AI | IONOS AI Model Hub (OpenAI-compatible API) |
| TTS | ElevenLabs Streaming API |
| Auth | JWT (HS256), bcrypt passwords |
| Billing | Stripe Checkout + Webhooks |
| Email | SMTP (any provider) |
| Frontend | Vanilla HTML/CSS/JS, dark glassmorphism UI |
| Storage | JSON flat files (`data/users.json`, `data/contexts.json`) |

---

## Prerequisites

- **Go 1.21** (`go version` to check)
- API keys for:
  - [IONOS AI Model Hub](https://cloud.ionos.com/managed/ai-model-hub)
  - [ElevenLabs](https://elevenlabs.io)
  - [Stripe](https://dashboard.stripe.com)
- An SMTP provider for verification emails (SendGrid, Postmark, AWS SES, Gmail, etc.)

---

## Local Development Setup

### 1. Clone the repo

```bash
git clone <your-repo-url>
cd ailanguagetutor
```

### 2. Configure environment variables

```bash
cp .env.example .env
```

Open `.env` and fill in all required values. See the [Environment Variables](#environment-variables) section below for details.

### 3. Install dependencies

```bash
GOTOOLCHAIN=local go mod tidy
```

### 4. Run the server

```bash
make run
```

Or manually:

```bash
set -a && . ./.env && set +a && GOTOOLCHAIN=local go run .
```

The server starts at **http://localhost:8080**.

> **Email verification in dev mode:** If `SMTP_HOST` is left empty, verification emails are skipped and the verification URL is printed to stdout. Copy it into your browser to complete signup without a mail server.

### 5. Create your admin account

Register at http://localhost:8080 using the email address set as `AdminEmail` in `store/store.go` (default: `anthony@csuitecode.com`). The admin account bypasses email verification and Stripe checkout and gets immediate free access.

---

## Environment Variables

Copy `.env.example` to `.env` and fill in the values below.

### Required

| Variable | Description |
|---|---|
| `JWT_SECRET` | Random secret for signing JWTs. Generate with: `openssl rand -hex 32` |
| `IONOS_API_KEY` | API key from IONOS AI Model Hub |
| `ELEVENLABS_API_KEY` | API key from ElevenLabs |
| `STRIPE_SECRET_KEY` | Stripe secret key (`sk_live_...` or `sk_test_...`) |
| `STRIPE_WEBHOOK_SECRET` | Stripe webhook signing secret (`whsec_...`) |
| `STRIPE_PRICE_ID` | ID of the subscription price in Stripe (`price_...`) |
| `APP_BASE_URL` | Public URL of the app, e.g. `https://yourdomain.com` (used in email links and Stripe redirects) |

### Email (SMTP)

| Variable | Default | Description |
|---|---|---|
| `SMTP_HOST` | _(empty)_ | SMTP server hostname. Leave empty to skip sending emails in dev. |
| `SMTP_PORT` | `587` | SMTP port (587 for STARTTLS, 465 for SSL) |
| `SMTP_USERNAME` | — | SMTP login username |
| `SMTP_PASSWORD` | — | SMTP login password |
| `EMAIL_FROM` | — | From address, e.g. `noreply@yourdomain.com` |

### Optional

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8080` | HTTP server port |
| `IONOS_BASE_URL` | `https://openai.inference.de-txl.ionos.com/v1` | IONOS API base URL |
| `IONOS_MODEL` | `mistral-small-24b` | AI model name |
| `ELEVENLABS_MODEL` | `eleven_multilingual_v2` | TTS model |
| `ELEVENLABS_VOICE_IT/ES/PT/FR/DE/JA/RU/RO/ZH` | Rachel (multilingual) | Voice ID per language |

---

## Stripe Setup

### 1. Create a product and price

In your [Stripe Dashboard](https://dashboard.stripe.com/products):
- Create a product (e.g. "LinguaAI Subscription")
- Add a recurring price: `$100/month`
- Copy the **Price ID** (`price_...`) → set as `STRIPE_PRICE_ID`

### 2. Set up a webhook

In your Stripe Dashboard → Developers → Webhooks:
- Add endpoint: `https://yourdomain.com/api/billing/webhook`
- Select these events:
  - `checkout.session.completed`
  - `customer.subscription.updated`
  - `customer.subscription.deleted`
  - `invoice.payment_failed`
  - `invoice.payment_succeeded`
- Copy the **Signing Secret** (`whsec_...`) → set as `STRIPE_WEBHOOK_SECRET`

### 3. Local webhook testing

Use the [Stripe CLI](https://stripe.com/docs/stripe-cli) to forward webhooks locally:

```bash
stripe listen --forward-to localhost:8080/api/billing/webhook
```

---

## User Registration Flow

1. User fills in username, email, password, and plan (trial or immediate)
2. Server creates account and sends a verification email
3. User clicks the verification link → server marks email as verified → redirects to Stripe Checkout
4. After payment, Stripe webhook activates the subscription
5. User can now sign in at `/`

> Admin accounts skip email verification and Stripe — they get immediate free access.

---

## Deployment

### Build the binary

```bash
make build
# Produces: bin/linguaai
```

### Option A — Systemd (Linux VPS)

1. **Copy the binary and static files to your server:**

```bash
scp bin/linguaai user@yourserver:/opt/linguaai/
scp -r static/ data/ user@yourserver:/opt/linguaai/
```

2. **Create a `.env` file on the server** at `/opt/linguaai/.env` with production values.

3. **Create a systemd service** at `/etc/systemd/system/linguaai.service`:

```ini
[Unit]
Description=LinguaAI Language Tutor
After=network.target

[Service]
Type=simple
User=www-data
WorkingDirectory=/opt/linguaai
EnvironmentFile=/opt/linguaai/.env
ExecStart=/opt/linguaai/linguaai
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

4. **Enable and start:**

```bash
sudo systemctl daemon-reload
sudo systemctl enable linguaai
sudo systemctl start linguaai
sudo systemctl status linguaai
```

5. **View logs:**

```bash
sudo journalctl -u linguaai -f
```

### Option B — Docker

1. **Create a `Dockerfile`:**

```dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN GOTOOLCHAIN=local go build -o linguaai .

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/linguaai .
COPY static/ static/
COPY data/ data/
EXPOSE 8080
CMD ["./linguaai"]
```

2. **Build and run:**

```bash
docker build -t linguaai .
docker run -d \
  --name linguaai \
  -p 8080:8080 \
  --env-file .env \
  -v $(pwd)/data:/app/data \
  linguaai
```

> Mount `data/` as a volume so user data survives container restarts.

### Option C — Render / Railway / Fly.io

These platforms support Go apps natively. Set environment variables via the platform dashboard and point the start command to `make run` or `./bin/linguaai`.

---

## Nginx Reverse Proxy

Put Nginx in front of the app for TLS termination and static file caching.

```nginx
server {
    listen 80;
    server_name yourdomain.com;
    return 301 https://$host$request_uri;
}

server {
    listen 443 ssl http2;
    server_name yourdomain.com;

    ssl_certificate     /etc/letsencrypt/live/yourdomain.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/yourdomain.com/privkey.pem;

    # Disable buffering for SSE streaming
    proxy_buffering off;
    proxy_cache off;

    location / {
        proxy_pass         http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header   Host              $host;
        proxy_set_header   X-Real-IP         $remote_addr;
        proxy_set_header   X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto $scheme;

        # Required for SSE (conversation streaming)
        proxy_read_timeout 120s;
        proxy_send_timeout 120s;
    }
}
```

Get a free TLS certificate with [Certbot](https://certbot.eff.org/):

```bash
sudo certbot --nginx -d yourdomain.com
```

---

## Project Structure

```
.
├── main.go                    # Server entry point, router setup
├── Makefile                   # run / build / tidy commands
├── go.mod
├── config/
│   └── config.go              # Environment-based configuration
├── store/
│   └── store.go               # In-memory user + session stores, JSON persistence
├── middleware/
│   └── auth.go                # JWT Bearer + Cookie auth middleware
├── handlers/
│   ├── auth.go                # Register, Login, Logout, Me, VerifyEmail
│   ├── billing.go             # Stripe Checkout, Webhook, Portal, Status
│   ├── conversation.go        # Session start, SSE message streaming, history
│   ├── tts.go                 # ElevenLabs TTS proxy
│   ├── meta.go                # GET /api/languages, GET /api/topics
│   ├── admin.go               # Admin user list + subscription management
│   ├── email.go               # SMTP email sending (verification emails)
│   └── helpers.go             # writeJSON helper
├── data/
│   ├── users.json             # Persisted user records (auto-created)
│   ├── contexts.json          # Per-user conversation context (auto-created)
│   └── languages.json / topics.json / contexts.json
└── static/
    ├── index.html             # Login / Register
    ├── dashboard.html         # Language + topic selection
    ├── conversation.html      # Chat UI with SSE + voice
    ├── profile.html           # Profile & billing
    ├── admin.html             # Admin panel
    ├── checkout-complete.html # Post-Stripe redirect handler
    ├── css/
    │   └── style.css
    └── js/
        ├── api.js             # Fetch wrapper
        ├── auth.js            # Auth page logic
        ├── dashboard.js       # Language/topic selection + banners
        ├── conversation.js    # SSE streaming, voice input, TTS
        ├── profile.js         # Subscription management
        └── admin.js           # Admin user management
```

---

## API Reference

### Auth (public)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/auth/register` | Create account + send verification email |
| `POST` | `/api/auth/login` | Sign in, returns JWT |
| `GET` | `/api/auth/verify-email?token=` | Verify email, redirect to Stripe |

### Auth (requires JWT)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/auth/logout` | Sign out |
| `GET` | `/api/auth/me` | Current user info |

### Billing (requires JWT)

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/billing/status` | Subscription status |
| `POST` | `/api/billing/checkout` | Create Stripe Checkout session |
| `POST` | `/api/billing/cancel` | Cancel subscription |
| `POST` | `/api/billing/portal` | Create Stripe Customer Portal session |

### Billing (public)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/billing/webhook` | Stripe webhook receiver |
| `GET` | `/api/billing/verify-checkout?session_id=` | Verify checkout after redirect |

### Conversation (requires JWT)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/conversation/start` | Start a new session |
| `POST` | `/api/conversation/message` | Send message, stream response (SSE) |
| `POST` | `/api/conversation/translate` | Translate text to English |
| `GET` | `/api/conversation/history/:sessionId` | Get session messages |

### Admin (requires JWT + admin role)

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/admin/users` | List all users |
| `PATCH` | `/api/admin/users/:id/subscription` | Set subscription status |

---

## Subscription Statuses

| Status | Meaning | Access |
|---|---|---|
| `trialing` | 7-day free trial active | Levels 1–3 only |
| `active` | Paid subscription | All 5 levels |
| `past_due` | Payment failed | All levels (grace period) |
| `cancelled` | Cancelled (may retain trial access) | Levels 1–3 until trial expires |
| `free` | Admin-granted free access | All 5 levels |
| `suspended` | Admin-revoked | None |
| _(empty)_ | No subscription set up | None |

---

## License

MIT
