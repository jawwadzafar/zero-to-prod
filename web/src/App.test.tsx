import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { Link } from './api';
import { App } from './App';
import { json, snipError, stubFetch } from './test/fakeFetch';

const godoc: Link = {
  slug: 'godoc',
  url: 'https://go.dev/doc/',
  clicks: 0,
  created_at: '2026-10-30T09:15:00Z',
  short_url: 'http://localhost:8080/godoc',
};

// Tests drive the UI the way a person does: find things by their label or
// role, type, click, then check what is on the screen.
async function enterKey(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText('API key'), 'snip_test');
  await user.click(screen.getByRole('button', { name: 'Continue' }));
}

describe('App', () => {
  it('asks for a key, then shows the empty state', async () => {
    stubFetch(json(200, { links: [] }));
    const user = userEvent.setup();
    render(<App />);

    expect(screen.getByRole('button', { name: 'Continue' })).toBeDisabled();
    await enterKey(user);

    expect(await screen.findByText(/No links yet/)).toBeInTheDocument();
  });

  it('creates a link and shows it at the top of the list', async () => {
    const fetchMock = stubFetch(json(200, { links: [] }), json(201, godoc));
    const user = userEvent.setup();
    render(<App />);
    await enterKey(user);
    await screen.findByText(/No links yet/);

    await user.type(screen.getByLabelText('Long URL'), 'https://go.dev/doc/');
    await user.type(screen.getByLabelText('Custom slug (optional)'), 'godoc');
    await user.click(screen.getByRole('button', { name: 'Create short link' }));

    expect(await screen.findByRole('link', { name: godoc.short_url })).toBeInTheDocument();
    expect(fetchMock).toHaveBeenLastCalledWith('/api/links', expect.objectContaining({ method: 'POST' }));
    expect(screen.getByLabelText('Long URL')).toHaveValue(''); // the form is ready for the next one
  });

  it("shows snip's message next to the slug field when the slug is taken", async () => {
    stubFetch(json(200, { links: [] }), snipError(409, 'slug_taken', 'that slug is already in use'));
    const user = userEvent.setup();
    render(<App />);
    await enterKey(user);
    await screen.findByText(/No links yet/);

    await user.type(screen.getByLabelText('Long URL'), 'https://go.dev/doc/');
    await user.type(screen.getByLabelText('Custom slug (optional)'), 'godoc');
    await user.click(screen.getByRole('button', { name: 'Create short link' }));

    expect(await screen.findByText('that slug is already in use')).toBeInTheDocument();
    expect(screen.getByLabelText('Custom slug (optional)')).toHaveAttribute('aria-invalid', 'true');
  });

  it('deletes a link after the user confirms', async () => {
    stubFetch(json(200, { links: [godoc] }), new Response(null, { status: 204 }));
    vi.stubGlobal('confirm', () => true);
    const user = userEvent.setup();
    render(<App />);
    await enterKey(user);

    await user.click(await screen.findByRole('button', { name: 'Delete godoc' }));

    expect(await screen.findByText(/No links yet/)).toBeInTheDocument();
  });

  it('goes back to the key form when snip rejects the key', async () => {
    stubFetch(snipError(401, 'unauthorized', 'missing or invalid API key'));
    const user = userEvent.setup();
    render(<App />);
    await enterKey(user);

    expect(await screen.findByRole('alert')).toHaveTextContent('did not accept that API key');
    expect(screen.getByLabelText('API key')).toHaveValue('');
  });

  it('offers to try again when the list fails to load', async () => {
    stubFetch(new TypeError('Failed to fetch'), json(200, { links: [godoc] }));
    const user = userEvent.setup();
    render(<App />);
    await enterKey(user);

    expect(await screen.findByText(/Could not reach snip/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Try again' }));

    expect(await screen.findByText('0 clicks')).toBeInTheDocument();
  });

  // Accessibility (chapter 16.7): where focus goes when the screen changes
  // under it, and what a screen reader is told.
  it('moves focus to the first field once the key is accepted', async () => {
    stubFetch(json(200, { links: [] }));
    const user = userEvent.setup();
    render(<App />);
    await enterKey(user);

    expect(screen.getByLabelText('Long URL')).toHaveFocus();
  });

  it('announces a new link without moving focus', async () => {
    stubFetch(json(200, { links: [] }), json(201, godoc));
    const user = userEvent.setup();
    render(<App />);
    await enterKey(user);
    await screen.findByText(/No links yet/);

    await user.type(screen.getByLabelText('Long URL'), 'https://go.dev/doc/');
    await user.click(screen.getByRole('button', { name: 'Create short link' }));

    expect(await screen.findByRole('status')).toHaveTextContent(`Created ${godoc.short_url}`);
    expect(screen.getByRole('button', { name: 'Create short link' })).toHaveFocus();
  });

  it('after a delete, announces it and puts focus on the list heading', async () => {
    stubFetch(json(200, { links: [godoc] }), new Response(null, { status: 204 }));
    vi.stubGlobal('confirm', () => true);
    const user = userEvent.setup();
    render(<App />);
    await enterKey(user);

    await user.click(await screen.findByRole('button', { name: 'Delete godoc' }));

    expect(await screen.findByRole('status')).toHaveTextContent('Deleted godoc');
    expect(screen.getByRole('heading', { name: 'Your links' })).toHaveFocus();
  });

  it('gives the key field focus again after "Forget key"', async () => {
    stubFetch(json(200, { links: [] }));
    const user = userEvent.setup();
    render(<App />);
    await enterKey(user);

    await user.click(screen.getByRole('button', { name: 'Forget key' }));

    expect(screen.getByLabelText('API key')).toHaveFocus();
    expect(screen.getByLabelText('API key')).toHaveAccessibleDescription(/snip keys create/);
  });
});
