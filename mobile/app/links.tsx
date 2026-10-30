// The links screen ("/links"): your links, newest first.
import { useCallback, useState } from 'react';
import {
  AccessibilityInfo,
  ActivityIndicator,
  Alert,
  FlatList,
  Pressable,
  Share,
  StyleSheet,
  Text,
  View,
} from 'react-native';
import * as Clipboard from 'expo-clipboard';
import { router, Stack, useFocusEffect } from 'expo-router';
import { ApiError, describeError, type Link } from '../src/api';
import { Button } from '../src/components/Button';
import { useClient, useSession } from '../src/session';
import { colors, MIN_TOUCH, shared } from '../src/theme';

// The three states a list can be in, and only those three (as in web/).
type ListState =
  | { status: 'loading' }
  | { status: 'error'; message: string }
  | { status: 'ready'; links: Link[] };

export default function LinksScreen() {
  const client = useClient();
  const { signOut } = useSession();
  const [list, setList] = useState<ListState>({ status: 'loading' });
  const [refreshing, setRefreshing] = useState(false);
  // One line of feedback after an action ("Copied …", or what went wrong).
  const [status, setStatus] = useState<string | null>(null);

  // Any call can fail with 401 (the key was revoked a minute ago, say). That
  // always means "ask for a key again": signing out flips the layout's guard,
  // which takes this screen away and shows the setup screen with the reason.
  const handleError = useCallback(
    (err: unknown): string | null => {
      if (err instanceof ApiError && err.code === 'unauthorized') {
        void signOut(describeError(err));
        return null;
      }
      return describeError(err);
    },
    [signOut],
  );

  const load = useCallback(
    async (signal?: AbortSignal) => {
      if (!client) return;
      try {
        const links = await client.listLinks(signal);
        setList({ status: 'ready', links });
      } catch (err) {
        if (signal?.aborted) return; // the screen lost focus; nobody to tell
        const message = handleError(err);
        if (message) setList({ status: 'error', message });
      }
    },
    [client, handleError],
  );

  // useFocusEffect is useEffect for screens: it runs when this screen comes
  // into view, including when the create form's modal closes over it, so a
  // new link shows up without any extra wiring. The cleanup cancels the
  // request if the screen goes out of view first.
  useFocusEffect(
    useCallback(() => {
      const controller = new AbortController();
      void load(controller.signal);
      return () => controller.abort();
    }, [load]),
  );

  async function handleRefresh() {
    setRefreshing(true);
    await load();
    setRefreshing(false);
  }

  // Tell everyone what happened: sighted users read the status line, and
  // screen readers say it out loud.
  function announce(message: string) {
    setStatus(message);
    AccessibilityInfo.announceForAccessibility(message);
  }

  async function handleCopy(link: Link) {
    await Clipboard.setStringAsync(link.short_url);
    announce(`Copied ${link.short_url}`);
  }

  async function handleShare(link: Link) {
    // The system share sheet: Messages, Mail, WhatsApp, whatever this phone
    // has. On iOS, message and url would both be shared, so send only message.
    await Share.share({ message: link.short_url });
  }

  function confirmDelete(link: Link) {
    // Alert.alert is the native dialog: buttons in the platform's order, and
    // style "destructive" turns Delete red on iOS.
    Alert.alert(`Delete ${link.slug}?`, `Anyone who opens ${link.short_url} will get a 404.`, [
      { text: 'Cancel', style: 'cancel' },
      { text: 'Delete', style: 'destructive', onPress: () => void handleDelete(link.slug) },
    ]);
  }

  async function handleDelete(slug: string) {
    if (!client) return;
    try {
      await client.deleteLink(slug);
      setList((prev) =>
        prev.status === 'ready' ? { status: 'ready', links: prev.links.filter((l) => l.slug !== slug) } : prev,
      );
      announce(`Deleted ${slug}`);
    } catch (err) {
      const message = handleError(err);
      if (message) announce(message);
    }
  }

  return (
    <View style={shared.screen}>
      {/* Header buttons for this screen. The layout sets the title; a screen
          can add to its own options like this, next to the code they call. */}
      <Stack.Screen
        options={{
          headerLeft: () => (
            <Button title="Forget key" variant="secondary" onPress={() => void signOut()} />
          ),
          headerRight: () => (
            <Button title="New" accessibilityLabel="New short link" onPress={() => router.push('/new')} />
          ),
        }}
      />
      {status && (
        <Text style={styles.status} accessibilityLiveRegion="polite">
          {status}
        </Text>
      )}
      {list.status === 'loading' && <ActivityIndicator style={styles.centre} accessibilityLabel="Loading your links" />}
      {list.status === 'error' && (
        <View style={styles.centre} accessibilityRole="alert">
          <Text style={shared.error}>{list.message}</Text>
          <Button title="Try again" onPress={() => void load()} />
        </View>
      )}
      {list.status === 'ready' && (
        // FlatList draws only the rows near the screen, not all 100: on a
        // phone, memory and frame time are the budget. keyExtractor gives
        // each row a stable identity (the slug), like key in web/'s list.
        <FlatList
          data={list.links}
          keyExtractor={(link) => link.slug}
          renderItem={({ item }) => (
            <LinkRow
              link={item}
              onCopy={() => void handleCopy(item)}
              onShare={() => void handleShare(item)}
              onDelete={() => confirmDelete(item)}
            />
          )}
          refreshing={refreshing}
          onRefresh={() => void handleRefresh()} // pull down to reload
          ItemSeparatorComponent={() => <View style={styles.separator} />}
          ListEmptyComponent={<Text style={styles.empty}>No links yet. Tap New to shorten your first one.</Text>}
        />
      )}
    </View>
  );
}

interface RowProps {
  link: Link;
  onCopy: () => void;
  onShare: () => void;
  onDelete: () => void;
}

function LinkRow({ link, onCopy, onShare, onDelete }: RowProps) {
  return (
    <View style={styles.row}>
      {/* The text is one big button that copies the short link. The Share and
          Delete buttons sit beside it, not inside it: a screen reader treats
          a touchable as one element, and would never reach buttons nested in it. */}
      <Pressable
        style={styles.rowText}
        accessibilityRole="button"
        accessibilityLabel={`${link.short_url}, ${link.clicks} ${link.clicks === 1 ? 'click' : 'clicks'}`}
        accessibilityHint="Copies the short link"
        onPress={onCopy}
      >
        <Text style={styles.slug}>{link.slug}</Text>
        <Text style={styles.url} numberOfLines={1}>
          {link.url}
        </Text>
        <Text style={shared.hint}>
          {link.clicks} {link.clicks === 1 ? 'click' : 'clicks'}
        </Text>
      </Pressable>
      <View style={styles.actions}>
        <Button title="Share" variant="secondary" accessibilityLabel={`Share ${link.slug}`} onPress={onShare} />
        <Button title="Delete" variant="danger" accessibilityLabel={`Delete ${link.slug}`} onPress={onDelete} />
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  centre: { padding: 24, gap: 12, alignItems: 'center' },
  status: { padding: 12, backgroundColor: colors.card, color: colors.text },
  row: { flexDirection: 'row', alignItems: 'center', gap: 8, padding: 12, backgroundColor: colors.card },
  rowText: { flex: 1, minHeight: MIN_TOUCH, justifyContent: 'center' },
  slug: { fontSize: 17, fontWeight: '600', color: colors.text },
  url: { color: colors.muted },
  actions: { gap: 8 },
  separator: { height: StyleSheet.hairlineWidth, backgroundColor: colors.border },
  empty: { padding: 24, textAlign: 'center', color: colors.muted },
});
