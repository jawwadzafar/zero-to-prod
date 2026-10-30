// The setup screen ("/"): where snip is, and your API key.
import { useState } from 'react';
import { KeyboardAvoidingView, Platform, ScrollView, Text, TextInput } from 'react-native';
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

  async function handleSave() {
    setSaving(true);
    setError(null);
    try {
      // After this, the layout's guard flips and the router shows the links.
      await signIn({ baseUrl: baseUrl.trim(), apiKey: apiKey.trim() });
    } catch {
      setError('Could not save the key on this phone. Try again.');
      setSaving(false);
    }
  }

  return (
    // The keyboard covers the bottom half of the screen; this moves the form up out of its way on iOS.
    <KeyboardAvoidingView style={shared.screen} behavior={Platform.OS === 'ios' ? 'padding' : undefined}>
      <ScrollView contentContainerStyle={shared.form} keyboardShouldPersistTaps="handled">
        {notice && (
          // accessibilityRole="alert" and the live region make screen readers
          // say it as soon as it appears (iOS and Android respectively).
          <Text style={shared.error} accessibilityRole="alert" accessibilityLiveRegion="polite">
            {notice}
          </Text>
        )}
        <Text style={shared.label}>snip address</Text>
        <TextInput
          style={shared.input}
          accessibilityLabel="snip address"
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
          value={apiKey}
          onChangeText={setApiKey}
          secureTextEntry // shows dots, and keeps the key out of the keyboard's suggestions
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
