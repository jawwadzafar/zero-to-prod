// The same colours as web/src/styles.css (light mode), so the app and the
// web UI look like one product. Every text colour passes 4.5:1 on its background.
import { StyleSheet } from 'react-native';

export const colors = {
  bg: '#f6f7f9',
  card: '#ffffff',
  text: '#1c2024',
  muted: '#5b6470',
  border: '#d6dae0', // decorative lines only
  inputBorder: '#868e98', // 3:1 against the card, so a field's edge is visible
  accent: '#c2410c',
  accentText: '#ffffff',
  danger: '#b42318',
};

// Apple asks for touch targets of at least 44×44 points; Android's guideline
// is 48dp. 44 is the floor here, and nothing has a fixed height, so a row
// grows when someone turns their system font size up.
export const MIN_TOUCH = 44;

export const shared = StyleSheet.create({
  screen: { flex: 1, backgroundColor: colors.bg },
  form: { padding: 16, gap: 8 },
  label: { color: colors.text, fontWeight: '600', marginTop: 8 },
  input: {
    minHeight: MIN_TOUCH,
    borderWidth: 1,
    borderColor: colors.inputBorder,
    borderRadius: 6,
    paddingHorizontal: 12,
    paddingVertical: 8,
    backgroundColor: colors.card,
    color: colors.text,
    fontSize: 16,
  },
  inputInvalid: { borderColor: colors.danger, borderWidth: 2 },
  hint: { color: colors.muted },
  error: { color: colors.danger },
});
