// Where the app keeps snip's address and your API key, and how every screen
// gets at them.
//
// The key goes in expo-secure-store, never in AsyncStorage. AsyncStorage is a
// plain file inside the app's folder: anyone with a backup of the phone, or a
// rooted one, can read it. Secure Store hands the value to the operating
// system's own vault for secrets: the Keychain on iOS, and on Android a value
// encrypted with a key held by the Android Keystore, which never leaves it.
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { Platform } from 'react-native';
import * as SecureStore from 'expo-secure-store';
import { createClient, type SnipClient } from './api';

// Names for the two stored values. Secure Store allows letters, digits, ".", "-" and "_".
const KEY_ITEM = 'snip.apiKey';
const URL_ITEM = 'snip.baseUrl';

export interface Session {
  baseUrl: string;
  apiKey: string;
}

/**
 * Where snip probably is while you develop. EXPO_PUBLIC_SNIP_URL wins if you
 * set it (Expo copies EXPO_PUBLIC_ variables into the app when it bundles).
 * Otherwise: the Android emulator is a separate virtual machine, so its
 * "localhost" is the emulator itself; Google reserves 10.0.2.2 inside it to
 * mean "the computer running the emulator". The iOS simulator shares your
 * computer's network, so localhost works there. A real phone needs your
 * computer's address on the Wi-Fi, set through EXPO_PUBLIC_SNIP_URL.
 */
export function defaultBaseUrl(): string {
  if (process.env.EXPO_PUBLIC_SNIP_URL) return process.env.EXPO_PUBLIC_SNIP_URL;
  return Platform.OS === 'android' ? 'http://10.0.2.2:8080' : 'http://localhost:8080';
}

export async function loadSession(): Promise<Session | null> {
  const [apiKey, baseUrl] = await Promise.all([
    SecureStore.getItemAsync(KEY_ITEM),
    SecureStore.getItemAsync(URL_ITEM), // not a secret, but one store keeps this short
  ]);
  return apiKey ? { apiKey, baseUrl: baseUrl ?? defaultBaseUrl() } : null;
}

export async function saveSession(session: Session): Promise<void> {
  await SecureStore.setItemAsync(URL_ITEM, session.baseUrl);
  await SecureStore.setItemAsync(KEY_ITEM, session.apiKey);
}

export async function forgetKey(): Promise<void> {
  await SecureStore.deleteItemAsync(KEY_ITEM);
  await SecureStore.deleteItemAsync(URL_ITEM);
}

// The same state for every screen, through React context: the setup screen
// signs in, and the links screen (or the layout) sees it at once.
interface SessionContextValue {
  loading: boolean; // true until Secure Store has answered, once, at startup
  session: Session | null;
  notice: string | null; // why we are asking for a key again, if we are
  signIn(session: Session): Promise<void>;
  signOut(notice?: string): Promise<void>;
}

const SessionContext = createContext<SessionContextValue | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const [loading, setLoading] = useState(true);
  const [session, setSession] = useState<Session | null>(null);
  const [notice, setNotice] = useState<string | null>(null);

  useEffect(() => {
    loadSession()
      .then(setSession)
      .catch(() => setSession(null)) // an unreadable store is the same as an empty one: ask again
      .finally(() => setLoading(false));
  }, []);

  const signIn = useCallback(async (next: Session) => {
    await saveSession(next);
    setNotice(null);
    setSession(next);
  }, []);

  const signOut = useCallback(async (why?: string) => {
    await forgetKey();
    setNotice(why ?? null);
    setSession(null);
  }, []);

  const value = useMemo(
    () => ({ loading, session, notice, signIn, signOut }),
    [loading, session, notice, signIn, signOut],
  );
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): SessionContextValue {
  const value = useContext(SessionContext);
  if (!value) throw new Error('useSession must be used inside <SessionProvider>');
  return value;
}

/**
 * A client for the saved session. null only for the moment between signing
 * out and the layout taking the screen away, so callers can simply do nothing.
 */
export function useClient(): SnipClient | null {
  const { session } = useSession();
  return useMemo(() => (session ? createClient(session.baseUrl, session.apiKey) : null), [session]);
}
