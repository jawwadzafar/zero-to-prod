import { Pressable, StyleSheet, Text } from 'react-native';
import { colors, MIN_TOUCH } from '../theme';

interface Props {
  title: string;
  onPress: () => void;
  // What a screen reader says, when the visible title alone isn't enough
  // ("Delete" is; "Delete godoc" is better, in a list of ten Delete buttons).
  accessibilityLabel?: string;
  variant?: 'primary' | 'secondary' | 'danger';
  disabled?: boolean;
}

// One button for the whole app, so every one of them gets the accessibility
// details right: a role, a label, a disabled state and a big enough target.
export function Button({ title, onPress, accessibilityLabel, variant = 'primary', disabled = false }: Props) {
  return (
    <Pressable
      accessibilityRole="button"
      accessibilityLabel={accessibilityLabel ?? title}
      accessibilityState={{ disabled }}
      disabled={disabled}
      onPress={onPress}
      style={({ pressed }) => [
        styles.base,
        styles[variant],
        (pressed || disabled) && styles.dim, // feedback that the press landed
      ]}
    >
      <Text style={[styles.text, textStyles[variant]]}>{title}</Text>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  base: {
    minHeight: MIN_TOUCH,
    minWidth: MIN_TOUCH,
    paddingHorizontal: 14,
    paddingVertical: 8,
    borderRadius: 6,
    alignItems: 'center',
    justifyContent: 'center',
  },
  primary: { backgroundColor: colors.accent },
  secondary: { borderWidth: 1, borderColor: colors.inputBorder, backgroundColor: colors.card },
  danger: { borderWidth: 1, borderColor: colors.danger, backgroundColor: colors.card },
  dim: { opacity: 0.6 },
  text: { fontSize: 16, fontWeight: '600' },
});

const textStyles = StyleSheet.create({
  primary: { color: colors.accentText },
  secondary: { color: colors.text },
  danger: { color: colors.danger },
});
