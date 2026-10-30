// Tests for the create form, opened over the list as it would be in the app.
import { fireEvent, renderRouter, screen } from 'expo-router/testing-library';
import RootLayout from '../app/_layout';
import SetupScreen from '../app/index';
import LinksScreen from '../app/links';
import NewLinkScreen from '../app/new';
import { forgetKey, saveSession } from '../src/session';
import { json, snipError, stubFetch } from '../src/test/fakeFetch';

const routes = { _layout: RootLayout, index: SetupScreen, links: LinksScreen, new: NewLinkScreen };

beforeEach(() => saveSession({ baseUrl: 'http://localhost:8080', apiKey: 'snip_test' }));
afterEach(() => forgetKey());

describe('create form', () => {
  it("shows snip's message under the slug when the slug is taken", async () => {
    const fetchMock = stubFetch(json(200, { links: [] }), snipError(409, 'slug_taken', 'that slug is already in use'));
    await renderRouter(routes);
    await fireEvent.press(await screen.findByRole('button', { name: 'New short link' }));

    await fireEvent.changeText(await screen.findByLabelText('Long URL'), 'https://go.dev/doc/');
    await fireEvent.changeText(screen.getByLabelText('Custom slug (optional)'), 'godoc');
    await fireEvent.press(screen.getByRole('button', { name: 'Create short link' }));

    expect(await screen.findByText('that slug is already in use')).toBeOnTheScreen();
    expect(screen).toHavePathname('/new'); // still open, so the user can pick another slug
    expect(fetchMock).toHaveBeenLastCalledWith(
      'http://localhost:8080/api/links',
      expect.objectContaining({ method: 'POST', body: '{"url":"https://go.dev/doc/","slug":"godoc"}' }),
    );
  });
});
