// The create form ("/new"), shown as a modal over the links (see _layout.tsx).
import { useState } from 'react';
import { AccessibilityInfo, KeyboardAvoidingView, Platform, ScrollView, Text, TextInput } from 'react-native';
import { router } from 'expo-router';
import { ApiError, describeError } from '../src/api';
import { Button } from '../src/components/Button';
import { useClient, useSession } from '../src/session';
import { shared } from '../src/theme';

type Field = 'url' | 'slug' | 'form';

// Which input an error belongs under, so the message appears where the
// mistake is. Codes come from snip's Error schema (the same as web/).
function fieldFor(err: unknown): Field {
  if (err instanceof ApiError) {
    if (err.code === 'invalid_url') return 'url';
    if (err.code === 'invalid_slug' || err.code === 'slug_taken') return 'slug';
  }
  return 'form';
}

export default function NewLinkScreen() {
  const client = useClient();
  const { signOut } = useSession();
  const [url, setUrl] = useState('');
  const [slug, setSlug] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<{ field: Field; message: string } | null>(null);

  async function handleCreate() {
    if (!client || saving) return;
    setSaving(true);
    setError(null);
    try {
      await client.createLink(url.trim(), slug.trim() || undefined);
      router.back(); // close the modal; the list reloads when it comes back into view
    } catch (err) {
      if (err instanceof ApiError && err.code === 'unauthorized') {
        await signOut(describeError(err)); // the layout closes this and the list, and asks for a key
        return;
      }
      const message = describeError(err);
      setError({ field: fieldFor(err), message });
      // Spoken at once on iOS and Android; the alert role alone doesn't make VoiceOver speak.
      AccessibilityInfo.announceForAccessibility(message);
      setSaving(false);
    }
  }

  const errorFor = (field: Field) =>
    error?.field === field ? (
      <Text style={shared.error} accessibilityRole="alert" accessibilityLiveRegion="polite">
        {error.message}
      </Text>
    ) : null;

  return (
    <KeyboardAvoidingView style={shared.screen} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <ScrollView contentContainerStyle={shared.form} keyboardShouldPersistTaps="handled">
        <Text style={shared.label}>Long URL</Text>
        <TextInput
          style={[shared.input, error?.field === 'url' && shared.inputInvalid]}
          accessibilityLabel="Long URL"
          value={url}
          onChangeText={setUrl}
          keyboardType="url" // a keyboard with "/" and ".com" on it
          autoCapitalize="none"
          autoCorrect={false}
          autoFocus
          placeholder="https://example.com/a/long/page"
        />
        {errorFor('url')}
        <Text style={shared.label}>Custom slug (optional)</Text>
        <TextInput
          style={[shared.input, error?.field === 'slug' && shared.inputInvalid]}
          accessibilityLabel="Custom slug (optional)"
          value={slug}
          onChangeText={setSlug}
          autoCapitalize="none"
          autoCorrect={false}
          placeholder="my-launch"
        />
        <Text style={shared.hint}>3–32 letters, digits, - or _. Leave it empty and snip picks one.</Text>
        {errorFor('slug')}
        {errorFor('form')}
        <Button
          title={saving ? 'Creating…' : 'Create short link'}
          onPress={() => void handleCreate()}
          disabled={saving || url.trim() === ''}
        />
      </ScrollView>
    </KeyboardAvoidingView>
  );
}
