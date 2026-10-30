# snip mobile — a phone app for snip

A small React Native app (Expo) for [snip](../snip), the link shortener you
build in Zero to Prod. It is the reference implementation for Part 18 (Mobile):
every snippet in those chapters is copied from here, and everything here is tested.

It does four things: saves snip's address and your API key (in the phone's
secure storage), lists your links (pull down to refresh), creates new ones,
and copies, shares or deletes them.

```text
app/_layout.tsx   the navigator: a stack, with screens guarded by "is a key saved?"
app/index.tsx     setup: snip's address and your API key
app/links.tsx     your links (tap to copy, Share, Delete)
app/new.tsx       the create form, shown as a modal
src/api.ts        a typed client for snip's API (mirrors web/src/api.ts)
src/session.tsx   the key in Secure Store, shared with every screen
```

## Run it against snip

1. Start snip and create a key (from `snip/`):

   ```bash
   docker compose up -d --build --wait
   docker compose exec api snip keys create me    # copy the snip_… key it prints
   ```

2. Start the app (from `mobile/`), then open it in **Expo Go** on your phone
   (scan the QR code), or press `a` for an Android emulator or `i` for the iOS
   simulator:

   ```bash
   npm ci
   npm start
   ```

3. Enter snip's address and the key. The app suggests one that works on the
   same computer: `http://10.0.2.2:8080` on the Android emulator (its name for
   your computer) and `http://localhost:8080` on the iOS simulator.

   **On a real phone**, localhost is the phone itself. Use your computer's
   address on the Wi-Fi (the phone must be on the same network), either typed
   in or set before starting:

   ```bash
   EXPO_PUBLIC_SNIP_URL=http://192.168.1.20:8080 npm start
   ```

   The short links snip returns still start with `http://localhost:8080`,
   because that is snip's `SNIP_BASE_URL` in `docker-compose.yml`. Change it
   to the same address if you want links you share from the phone to work
   elsewhere.

## Check it

```bash
npm run typecheck   # tsc --noEmit
npm test            # Jest: the API client, the links screen and the create form, with a fake fetch
npm run bundle      # builds the Android JavaScript bundle into dist/, no device needed
```

CI runs all three on every push (`mobile` in `.github/workflows/ci.yml`).
