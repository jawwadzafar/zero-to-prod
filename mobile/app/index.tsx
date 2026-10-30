// The setup screen ("/"): where snip is, and your API key.
import { useEffect, useState } from 'react';
import { AccessibilityInfo, KeyboardAvoidingView, Platform, ScrollView, Text, TextInput } from 'react-native';
import { Button } from '../src/components/Button';
import { defaultBaseUrl, useSession } from '../src/session';
import { shared } from '../src/theme';

export default function SetupScreen() {
  const { notice, signIn } = useSession();
  const [baseUrl, setBaseUrl] = useState(defaultBaseUrl);
  const [apiKey, setApiKey] = useState('');
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const ready = baseUrl.trim() !== '' && apiKey.trim() !== '' && !saving;

  // Say why we are asking for a key again. The alert role below doesn't make
  // VoiceOver speak on its own; an announcement does, on both platforms.
  useEffect(() => {
    if (notice) AccessibilityInfo.announceForAccessibility(notice);
  }, [notice]);

  async function handleSave() {
    setSaving(true);
    setError(null);
    try {
      // After this, the layout's guard flips and the router shows the links.
      await signIn({ baseUrl: baseUrl.trim(), apiKey: apiKey.trim() });
    } catch {
      const message = 'Could not save the key on this phone. Try again.';
      setError(message);
      AccessibilityInfo.announceForAccessibility(message);
      setSaving(false);
    }
  }

  return (
    // The keyboard covers the bottom half of the screen; this moves the form up out of its way on iOS.
    <KeyboardAvoidingView style={shared.screen} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <ScrollView contentContainerStyle={shared.form} keyboardShouldPersistTaps="handled">
        {notice && (
          // The alert role tells a screen reader this is an important message
          // when it reaches it; the live region makes Android's TalkBack say it
          // when it appears. The announcement above covers iOS too.
          <Text style={shared.error} accessibilityRole="alert" accessibilityLiveRegion="polite">
            {notice}
          </Text>
        )}
        <Text style={shared.label}>snip address</Text>
        <TextInput
          style={shared.input}
          accessibilityLabel="snip address"
          testID="base-url" // for the end-to-end test (e2e/snip.yaml); screen readers don't see it
          value={baseUrl}
          onChangeText={setBaseUrl}
          keyboardType="url"
          autoCapitalize="none"
          autoCorrect={false}
          placeholder="http://192.168.1.20:8080"
        />
        <Text style={shared.hint}>
          On a real phone, use your computer&apos;s address on the Wi-Fi, not localhost.
        </Text>
        <Text style={shared.label}>API key</Text>
        <TextInput
          style={shared.input}
          accessibilityLabel="API key"
          testID="api-key"
          value={apiKey}
          onChangeText={setApiKey}
          secureTextEntry // shows dots, and keeps the key out of the keyboard's suggestions
          // An API key isn't a website password: ask iOS not to offer to save it in Passwords.
          textContentType="none"
          autoComplete="off"
          autoCapitalize="none"
          autoCorrect={false}
          placeholder="snip_…"
          onSubmitEditing={() => ready && void handleSave()}
        />
        <Text style={shared.hint}>
          Create one with snip keys create NAME. It is kept in this phone&apos;s secure storage (the iOS Keychain or
          the Android Keystore) until you choose Forget key.
        </Text>
        {error && (
          <Text style={shared.error} accessibilityRole="alert" accessibilityLiveRegion="polite">
            {error}
          </Text>
        )}
        <Button title={saving ? 'Saving…' : 'Save and continue'} onPress={() => void handleSave()} disabled={!ready} />
      </ScrollView>
    </KeyboardAvoidingView>
  );
}
