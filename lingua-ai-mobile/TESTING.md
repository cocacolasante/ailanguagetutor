# Fluentica AI — Mobile App Testing Guide

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Node.js | 18+ | https://nodejs.org |
| npm | 9+ | bundled with Node |
| Expo CLI | latest | `npm install -g expo-cli` |
| Expo Go app | latest | iOS App Store / Google Play |

Optional (for native builds):
- Xcode 15+ (macOS only, for iOS Simulator)
- Android Studio + Android SDK (for Android Emulator)
- EAS CLI: `npm install -g eas-cli`

---

## 1. Local Setup

```bash
cd lingua-ai-mobile
npm install
```

Create a `.env` file (copy from `.env.example`):

```bash
cp .env.example .env
```

Set the API base URL in `.env`:

```
EXPO_PUBLIC_API_BASE_URL=https://fluentica.app
```

Use `http://localhost:8080` if running the Go backend locally.

---

## 2. Running the App

```bash
npm start          # tunnel mode — works from any network (phone + laptop don't need same WiFi)
npm run start:lan  # LAN mode — faster, but phone must be on same WiFi as your Mac
```

`npm start` uses `--tunnel` via ngrok, which is the most reliable way to connect a physical device regardless of network.

---

## 3. Testing on a Physical Device (Easiest)

1. Install **Expo Go** on your phone (iOS or Android)
2. Run `npm start`
3. **iOS**: Open Camera app → scan the QR code in the terminal
4. **Android**: Open Expo Go → tap "Scan QR Code" → scan

> The app hot-reloads on every file save. No rebuild needed for JS/TS changes.

---

## 4. Testing on iOS Simulator (macOS only)

```bash
npx expo start --ios
```

Or press **`i`** in the terminal after `npm start`.

Requires Xcode and the iOS Simulator installed. First launch takes ~2 min to build.

---

## 5. Testing on Android Emulator

1. Open Android Studio → Virtual Device Manager → start an emulator
2. Run:
```bash
npx expo start --android
```
Or press **`a`** in the terminal after `npm start`.

---

## 6. Key Screens to Test

| Screen | How to reach | What to check |
|--------|-------------|---------------|
| Login / Register | App launch | Auth flow, error states |
| Home (Dashboard) | After login | Streak, FP, language progress |
| Practice Hub | Practice tab (📚) | Language/level chips, 4 mode cards |
| Vocab flashcards | Practice → Vocabulary | Check answer, result card, progress bar |
| Sentence builder | Practice → Sentences | Submit, feedback card, completion screen |
| Listening | Practice → Listening | Play audio, multiple choice, score screen |
| Writing coach | Practice → Writing Coach | SSE streaming, End Session modal |
| Improve | Improve tab (🎯) | Weak areas, mistakes, recent sessions |
| Session Detail | Improve → tap a record | Full AI summary, vocabulary, corrections |
| Leaderboard | Leaderboard tab (🏆) | Rankings, pull-to-refresh |
| Profile | Profile tab (👤) | Subscription status, Stripe portal |
| Conversation | Home → Start Conversation | Full SSE chat, TTS playback, End session |

---

## 7. Testing Push Notifications (Daily Streak Reminders)

### On Device (physical phone required — simulators have limited notification support)

1. Open the app on a real device via Expo Go
2. Log in — on the **Home** screen, a permission prompt appears asking to allow notifications
3. **Allow** notifications
4. A daily reminder is scheduled for **8:00 PM** local time

#### Force-trigger for testing (change the time temporarily)

In `src/utils/notifications.ts`, change `REMINDER_HOUR` / `REMINDER_MINUTE` to a time 1–2 minutes in the future:

```typescript
const REMINDER_HOUR = 14;   // e.g. 2:00 PM
const REMINDER_MINUTE = 30;
```

Save → Expo hot-reloads → navigate to the Home tab to re-trigger the schedule → wait for the notification.

#### What to verify

- [ ] Permission prompt appears on first Home screen visit
- [ ] Notification fires at the scheduled time with the correct streak count in the message
- [ ] Tapping the notification opens the app and navigates to the Home tab
- [ ] Completing any practice session (Vocab / Sentences / Listening / Writing / Conversation) cancels today's reminder
- [ ] After cancellation, the notification does **not** fire again that day
- [ ] Next app open re-schedules the notification for the following day

#### iOS Simulator note
iOS Simulator supports local notifications but will show a permission dialog. The notification fires in the notification center — swipe down from the top to see it.

#### Android Emulator note
Android 13+ requires an additional `POST_NOTIFICATIONS` runtime permission. The emulator should prompt automatically when the app requests it.

---

## 8. Backend (Go server)

The app talks to the backend for all data. Run it locally with:

```bash
# From the project root
docker compose up -d --build
```

Backend will be available at `http://localhost:8080`. Update your `.env`:

```
EXPO_PUBLIC_API_BASE_URL=http://localhost:8080
```

> For physical device testing with a local server, use your machine's LAN IP instead of `localhost`:
> ```
> EXPO_PUBLIC_API_BASE_URL=http://192.168.1.x:8080
> ```

---

## 9. Production Build (EAS)

To test an optimised build (no Expo Go dependency):

```bash
# Login to Expo account
eas login

# Build for iOS (TestFlight / simulator)
eas build --platform ios --profile preview

# Build for Android (APK)
eas build --platform android --profile preview
```

Requires an `eas.json` — create one with `eas build:configure`.

---

## 10. Troubleshooting

| Problem | Fix |
|---------|-----|
| Metro bundler error | `npx expo start --clear` to reset the cache |
| API calls fail | Check `EXPO_PUBLIC_API_BASE_URL` in `.env` and ensure backend is running |
| Push notification not appearing | Ensure permission was granted; check device Do Not Disturb settings |
| Audio not playing | Make sure device is not on silent; TTS requires a network connection |
| SSE streaming hangs | Verify backend is running; check that JWT token is valid (re-login) |
| TypeScript errors | Run `npx tsc --noEmit` to see all errors |
