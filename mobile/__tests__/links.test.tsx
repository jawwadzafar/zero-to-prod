// Tests for the links screen. They render the real routes from app/ inside
// expo-router's test harness (renderRouter), so navigation is real too; only
// fetch and Secure Store are fakes.
import { Alert } from 'react-native';
import { fireEvent, renderRouter, screen } from 'expo-router/testing-library';
import RootLayout from '../app/_layout';
import SetupScreen from '../app/index';
import LinksScreen from '../app/links';
import NewLinkScreen from '../app/new';
import type { Link } from '../src/api';
import { forgetKey, saveSession } from '../src/session';
import { json, snipError, stubFetch } from '../src/test/fakeFetch';

const godoc: Link = {
  slug: 'godoc',
  url: 'https://go.dev/doc/',
  clicks: 3,
  created_at: '2026-10-30T09:15:00Z',
  short_url: 'http://localhost:8080/godoc',
};

const routes = { _layout: RootLayout, index: SetupScreen, links: LinksScreen, new: NewLinkScreen };

// A key is already saved, as if the person set the app up yesterday.
beforeEach(() => saveSession({ baseUrl: 'http://localhost:8080', apiKey: 'snip_test' }));
afterEach(() => forgetKey());

describe('links screen', () => {
  it('opens on the links when a key is saved, and lists them', async () => {
    stubFetch(json(200, { links: [godoc] }));
    await renderRouter(routes);

    expect(await screen.findByText('godoc')).toBeOnTheScreen();
    expect(screen.getByText('https://go.dev/doc/')).toBeOnTheScreen();
    expect(screen.getByText('3 clicks')).toBeOnTheScreen();
    expect(screen).toHavePathname('/links');
  });

  it('deletes a link after the user confirms', async () => {
    const fetchMock = stubFetch(json(200, { links: [godoc] }), new Response(null, { status: 204 }));
    // Press "Delete" in the native dialog as soon as it opens.
    const alert = jest.spyOn(Alert, 'alert').mockImplementation((_title, _message, buttons) => {
      buttons?.find((b) => b.text === 'Delete')?.onPress?.();
    });
    await renderRouter(routes);

    await fireEvent.press(await screen.findByRole('button', { name: 'Delete godoc' }));

    expect(alert).toHaveBeenCalledWith('Delete godoc?', expect.any(String), expect.any(Array));
    expect(await screen.findByText(/No links yet/)).toBeOnTheScreen();
    expect(fetchMock).toHaveBeenLastCalledWith(
      'http://localhost:8080/api/links/godoc',
      expect.objectContaining({ method: 'DELETE' }),
    );
    alert.mockRestore();
  });

  it('goes back to the setup screen, saying why, when snip rejects the key', async () => {
    stubFetch(snipError(401, 'unauthorized', 'missing or invalid API key'));
    await renderRouter(routes);

    expect(await screen.findByText(/did not accept that API key/)).toBeOnTheScreen();
    expect(screen).toHavePathname('/');
    expect(screen.getByLabelText('API key')).toBeOnTheScreen();
  });
});
