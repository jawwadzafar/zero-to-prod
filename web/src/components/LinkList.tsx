import type { Link } from '../api';

interface Props {
  links: Link[];
  onDelete: (slug: string) => void;
}

const dateFormat = new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' });

export function LinkList({ links, onDelete }: Props) {
  if (links.length === 0) {
    return <p className="empty">No links yet. Shorten your first one above.</p>;
  }

  return (
    <ul className="links">
      {/* key: the slug is unique and stable, so React can tell which row is which
          when one is added or removed (chapter 16.5). */}
      {links.map((link) => (
        <li key={link.slug} className="link">
          <div className="link-main">
            {/* A new tab: leaving this page would also forget the key. */}
            <a href={link.short_url} className="short" target="_blank" rel="noreferrer">
              {link.short_url}
            </a>
            <span className="long" title={link.url}>
              {link.url}
            </span>
          </div>
          <div className="link-meta">
            <span>
              {link.clicks} {link.clicks === 1 ? 'click' : 'clicks'}
            </span>
            <time dateTime={link.created_at}>{dateFormat.format(new Date(link.created_at))}</time>
            <button
              type="button"
              className="danger"
              aria-label={`Delete ${link.slug}`}
              onClick={() => {
                if (window.confirm(`Delete ${link.short_url}? Anyone who clicks it will get a 404.`)) {
                  onDelete(link.slug);
                }
              }}
            >
              Delete
            </button>
          </div>
        </li>
      ))}
    </ul>
  );
}
