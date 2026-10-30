// Jest uses this file in place of expo-secure-store in every test (a
// __mocks__ folder next to node_modules does that automatically). There is
// no Keychain or Keystore in Node, so a Map stands in for the phone's vault.
const items = new Map<string, string>();

export async function getItemAsync(key: string): Promise<string | null> {
  return items.get(key) ?? null;
}

export async function setItemAsync(key: string, value: string): Promise<void> {
  items.set(key, value);
}

export async function deleteItemAsync(key: string): Promise<void> {
  items.delete(key);
}
