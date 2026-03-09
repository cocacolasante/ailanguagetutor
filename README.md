# LinguaAI — AI-Powered Language Tutor

A real-time conversational language tutor powered by an LLM backend, ElevenLabs text-to-speech, Web Speech API voice input, and Stripe subscription billing.

---

## Features

- **Live conversation practice** — Streamed AI responses via Server-Sent Events (SSE)
- **3 languages** — Italian, Spanish, Portuguese
- **5 proficiency levels** — Beginner through Fluent, each with distinct teaching styles
- **50+ curated topics** — Organized across 8 categories: Everyday Life, Social, Travel & Leisure, Health & Learning, Professional, Role-Play Scenarios, Immersion Mode, Cultural Language Learning, Grammar & Skills, and AI Travel Mode
- **5 tutor personalities** — Professor, Friendly Partner, Bartender, Business Executive, Travel Guide
- **Dedicated practice modes** — Vocabulary builder, sentence construction, listening comprehension, and writing coach
- **AI improvement analysis** — Personalized feedback on your weakest areas
- **Voice I/O** — ElevenLabs TTS playback + Web Speech API voice input
- **Translation assist** — Inline translation of any AI message
- **Gamification** — Fluency Points (FP), daily streaks, 15 achievement badges, and a global leaderboard
- **Conversation memory** — Rolling context across sessions per user/language/level
- **Stripe billing** — 7-day free trial or immediate subscription; Customer Portal for self-service
- **Email verification** — New users verify their address before accessing the platform
- **Password reset** — Self-service forgot/reset password via email
- **Admin panel** — Manage subscriptions, invite users, grant/revoke access, delete accounts

---

## Tech Stack

| Layer | Technology |
|---|---|
| Backend | Go 1.21, [chi](https://github.com/go-chi/chi) router |
| AI | IONOS AI Model Hub (OpenAI-compatible API), model: `mistral-small-24b` |
| TTS | ElevenLabs Streaming API (`eleven_multilingual_v2`) |
| Auth | JWT (HS256), bcrypt passwords |
| Billing | Stripe Checkout + Webhooks |
| Email | SMTP (any provider) |
| Frontend | Vanilla HTML/CSS/JS, dark glassmorphism UI |
| Storage | PostgreSQL (auto-migrated on startup) |
| Deployment | Docker Compose |

---

## Prerequisites

- **Docker & Docker Compose**
- API keys for:
  - [IONOS AI Model Hub](https://cloud.ionos.com/managed/ai-model-hub)
  - [ElevenLabs](https://elevenlabs.io)
  - [Stripe](https://dashboard.stripe.com)
- An SMTP provider for verification emails (SendGrid, Postmark, AWS SES, Gmail, etc.)
- A PostgreSQL database

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

### 3. Build and run

```bash
docker compose up -d --build
```

The `--build` flag is required whenever Go code or static files change (Docker bakes them into the image at build time).

The server starts at **http://localhost:8080**.

> **Email verification in dev mode:** If `SMTP_HOST` is left empty, verification emails are skipped and the verification URL is printed to stdout. Copy it into your browser to complete signup without a mail server.

### 4. Create your admin account

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
| `DATABASE_URL` | PostgreSQL connection string, e.g. `postgres://user:pass@host:5432/dbname` |

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
| `ELEVENLABS_VOICE_IT/ES/PT` | Rachel (multilingual) | Voice ID per language |

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

## Project Structure

```
.
├── main.go                    # Server entry point, router setup
├── docker-compose.yml
├── Dockerfile
├── go.mod
├── config/
│   └── config.go              # Environment-based configuration
├── store/
│   └── store.go               # PostgreSQL user store + in-memory session/context/history stores
├── middleware/
│   └── auth.go                # JWT Bearer + Cookie auth middleware
├── handlers/
│   ├── auth.go                # Register, Login, Logout, Me, VerifyEmail, ForgotPassword, ResetPassword
│   ├── billing.go             # Stripe Checkout, Webhook, Portal, Status
│   ├── conversation.go        # Session start/end, SSE message streaming, history, translate
│   ├── gamification.go        # Stats, leaderboard, records, badges, mistakes
│   ├── vocab.go               # Vocabulary practice sessions
│   ├── sentences.go           # Sentence construction practice
│   ├── listening.go           # Listening comprehension sessions
│   ├── writing.go             # Writing coach sessions
│   ├── agent.go               # AI agent conversation URL helper
│   ├── tts.go                 # ElevenLabs TTS proxy
│   ├── meta.go                # GET /api/languages, /api/topics, /api/personalities
│   ├── admin.go               # Admin user management + subscription controls
│   ├── email.go               # SMTP email sending
│   └── helpers.go             # writeJSON helper
├── data/
│   └── conv_history.json      # Conversation records (auto-created, max 10 per user)
└── static/
    ├── index.html             # Login / Register
    ├── dashboard.html         # Language + topic + personality selection, stats widgets
    ├── conversation.html      # Chat UI with SSE + voice
    ├── summary.html           # Post-conversation summary (FP, badges, AI analysis)
    ├── leaderboard.html       # Global FP leaderboard (public)
    ├── vocab.html             # Vocabulary builder
    ├── sentences.html         # Sentence construction
    ├── listening.html         # Listening comprehension
    ├── writing.html           # Writing coach
    ├── improvement.html       # AI improvement analysis
    ├── profile.html           # Profile & billing
    ├── admin.html             # Admin panel
    ├── reset-password.html    # Password reset
    ├── checkout-complete.html # Post-Stripe redirect handler
    ├── css/
    │   └── style.css          # Design system (dark, glassmorphism)
    └── js/
        ├── api.js             # Fetch wrapper
        ├── auth.js            # Auth page logic + checkout redirect
        ├── dashboard.js       # Stats/streak/FP widgets, personality selection
        ├── conversation.js    # SSE streaming, voice input, TTS playback
        ├── summary.js         # Post-conversation summary rendering
        ├── leaderboard.js     # Public leaderboard
        ├── vocab.js           # Vocabulary session logic
        ├── sentences.js       # Sentence construction logic
        ├── listening.js       # Listening comprehension logic
        ├── writing.js         # Writing coach logic
        ├── improvement.js     # AI improvement analysis
        ├── profile.js         # Subscription management, Stripe portal
        └── admin.js           # Admin user management + subscription controls
```

---

## API Reference

### Auth (public)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/auth/register` | Create account + send verification email |
| `POST` | `/api/auth/login` | Sign in, returns JWT |
| `GET` | `/api/auth/verify-email?token=` | Verify email, redirect to Stripe |
| `POST` | `/api/auth/forgot-password` | Send password reset email |
| `POST` | `/api/auth/reset-password` | Reset password with token |

### Auth (requires JWT)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/auth/logout` | Sign out |
| `GET` | `/api/auth/me` | Current user info |
| `PATCH` | `/api/user/preferences` | Update user preferences |

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
| `POST` | `/api/conversation/end` | End session, generate AI summary, award FP |
| `GET` | `/api/conversation/history/{sessionId}` | Get session messages |

### Practice Modes (requires JWT)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/vocab/session` | Start vocabulary session |
| `POST` | `/api/vocab/check` | Check vocab answer |
| `POST` | `/api/vocab/word-result` | Record word result |
| `POST` | `/api/vocab/complete` | Complete vocab session |
| `POST` | `/api/sentences/session` | Start sentence construction session |
| `POST` | `/api/sentences/check` | Check sentence answer |
| `POST` | `/api/sentences/complete` | Complete sentence session |
| `POST` | `/api/listening/session` | Start listening comprehension session |
| `POST` | `/api/listening/complete` | Complete listening session |
| `POST` | `/api/writing/session` | Start writing coach session |
| `POST` | `/api/writing/message` | Send writing message |
| `POST` | `/api/writing/complete` | Complete writing session |

### Gamification (requires JWT)

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/user/stats` | Streak, FP, achievements, recent conversations |
| `GET` | `/api/user/mistakes` | Common mistake analysis |
| `GET` | `/api/conversation/records` | User's last 10 conversation records |
| `GET` | `/api/conversation/records/{id}` | Single conversation record |
| `GET` | `/api/badges` | All available achievement badges |

### Gamification (public)

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/leaderboard` | Top 50 users by total FP |

### Meta (public)

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/languages` | Available languages |
| `GET` | `/api/topics` | Available topics |
| `GET` | `/api/personalities` | Available tutor personalities |

### TTS (requires JWT)

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/tts` | Proxy ElevenLabs TTS |

### Admin (requires JWT + admin role)

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/admin/users` | List all users |
| `PATCH` | `/api/admin/users/{id}/subscription` | Set subscription status |
| `PATCH` | `/api/admin/users/{id}/approval` | Approve/revoke user |
| `POST` | `/api/admin/invite-user` | Invite a new user by email |
| `DELETE` | `/api/admin/users/{id}` | Delete a user |

---

## Gamification System

- **Fluency Points (FP)**: Earned at the end of each conversation. Formula: `min(100, userMsgCount*3 + level*5)`, minimum 5 per session.
- **Language Level**: `LanguageFP[lang] / 500 + 1`, capped at level 20.
- **Daily Streak**: Increments if last activity was yesterday; resets to 1 otherwise.
- **Achievements**: 15 badges checked automatically after each session (e.g. first conversation, streak milestones, FP thresholds).
- **Leaderboard**: Public ranking of top 50 users by total FP.

---

## Subscription Statuses

| Status | Meaning | Access |
|---|---|---|
| `trialing` | 7-day free trial active | Levels 1–3 only |
| `active` | Paid subscription | All 5 levels |
| `past_due` | Payment failed | All levels (grace period) |
| `cancelled` | Cancelled | Levels 1–3 until trial expires |
| `free` | Admin-granted free access | All 5 levels |
| `suspended` | Admin-revoked | None |
| _(empty)_ | No subscription set up | None |

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

## License

MIT
