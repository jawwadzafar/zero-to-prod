// The root layout: expo-router renders this around every screen in app/.
// Each file in app/ is a route (index.tsx is "/", links.tsx is "/links");
// this file decides how they are shown: a stack, where new screens slide in
// on top of old ones and Back pops them off.
import { useEffect } from 'react';
import { SplashScreen, Stack } from 'expo-router';
import { SessionProvider, useSession } from '../src/session';

// Keep the splash screen up until we know whether a key is saved. Without
// this, the router hides it as soon as this layout mounts, and you'd see a
// blank screen for the moment Secure Store takes to answer.
SplashScreen.preventAutoHideAsync();

export default function RootLayout() {
  return (
    <SessionProvider>
      <RootStack />
    </SessionProvider>
  );
}

function RootStack() {
  const { loading, session } = useSession();

  useEffect(() => {
    if (!loading) SplashScreen.hideAsync();
  }, [loading]);

  // Nothing to show until Secure Store says whether a key is saved; the
  // splash screen (held above) covers this moment.
  if (loading) return null;

  // Stack.Protected lets you visit a screen only while its guard is true.
  // With no key, only the setup screen is open; with one, only the links and
  // the create form are. When the guard flips (you save a key, or snip
  // rejects it), the router sends you to redirectTo. So no screen has to
  // navigate after signing in or out, and Back can't return to a screen you
  // are no longer allowed to see.
  return (
    <Stack>
      <Stack.Protected guard={!session} redirectTo="/links">
        <Stack.Screen name="index" options={{ title: 'Connect to snip' }} />
      </Stack.Protected>
      <Stack.Protected guard={!!session} redirectTo="/">
        <Stack.Screen name="links" options={{ title: 'Your links' }} />
        {/* A modal: it slides up over the list and is dismissed, not navigated back from. */}
        <Stack.Screen name="new" options={{ title: 'Shorten a link', presentation: 'modal' }} />
      </Stack.Protected>
    </Stack>
  );
}
