# LinguaAI Mobile — Claude Code Context

## Backend API base
Production: https://fluentica.app
Local dev:  http://localhost:8080

All API routes are prefixed with /api

## Auth
- POST /api/auth/register
- POST /api/auth/login  → returns { token: string, user: {...} }
- POST /api/auth/logout
- GET  /api/auth/me
- GET  /api/auth/verify-email?token=
- POST /api/auth/forgot-password
- POST /api/auth/reset-password
- PATCH /api/user/preferences

JWT stored in expo-secure-store under key "auth_token".
Sent as: Authorization: Bearer <token>

## Conversation (SSE streaming)
- POST /api/conversation/start       → { sessionId }
- POST /api/conversation/message     → SSE stream of text tokens
- POST /api/conversation/translate
- POST /api/conversation/end         → { summary, fluencyPoints, badges }
- GET  /api/conversation/history/:sessionId

SSE implementation: use react-native-sse package.
EventSource does NOT exist in React Native — never use browser EventSource.

## Practice modes
- POST /api/vocab/session + /check + /word-result + /complete
- POST /api/sentences/session + /check + /complete
- POST /api/listening/session + /complete
- POST /api/writing/session + /message + /complete

## Gamification
- GET /api/user/stats       → { streak, totalFP, languageFP, achievements, recentConversations }
- GET /api/user/mistakes
- GET /api/conversation/records
- GET /api/conversation/records/:id
- GET /api/badges
- GET /api/leaderboard

## Billing
- GET  /api/billing/status
- POST /api/billing/checkout    → { url }  (Stripe hosted page — open in browser)
- POST /api/billing/cancel
- POST /api/billing/portal      → { url }
- POST /api/billing/webhook     (server-to-server only)
- GET  /api/billing/verify-checkout?session_id=

Subscription statuses: trialing | active | past_due | cancelled | free | suspended
trialing + cancelled → levels 1-3 only
active + free → all 5 levels

## TTS
- POST /api/tts  → ElevenLabs audio stream proxy
  Body: { text: string, language: "it" | "es" | "pt" }
  Response: audio/mpeg stream
  Play with expo-av Audio.Sound

## Meta (public, no auth needed)
- GET /api/languages
- GET /api/topics
- GET /api/personalities

## Gamification formula
FP per session = min(100, messageCount * 3 + level * 5), min 5
Language level = floor(languageFP[lang] / 500) + 1, max 20
15 achievement badges checked after each session

## Design system
Dark theme, glassmorphism-inspired but adapted for mobile readability.
Primary accent: indigo/violet (#6366f1)
Background: dark slate (#0f0f1a)
Card surface: semi-transparent dark (#1e1e2e with opacity)
Success: emerald (#10b981)
Text primary: white
Text secondary: slate-400

## Important rules
1. SSE streaming → react-native-sse only, never browser EventSource
2. JWT → expo-secure-store only, never AsyncStorage
3. Audio playback → expo-av, Audio.Sound
4. Microphone → expo-av Audio.Recording
5. Navigation → Expo Router (file-based), never React Navigation directly
6. State → Zustand for auth/session, TanStack Query for all server data
7. No backend logic in the app — all business logic stays in the Go backend
8. Stripe billing → open Stripe Checkout URL in system browser via expo-linking
   Do NOT implement in-app purchases for web-originated subscriptions
9. Type everything — no `any` unless absolutely unavoidable
