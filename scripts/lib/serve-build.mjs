// A static server for build/, the way GitHub Pages serves it under
// /zero-to-prod/. Used by the checks and tools that drive a real browser.
import fs from 'node:fs';
import http from 'node:http';
import path from 'node:path';

const types = {'.html': 'text/html', '.js': 'text/javascript', '.css': 'text/css', '.json': 'application/json', '.svg': 'image/svg+xml', '.png': 'image/png', '.woff2': 'font/woff2'};

/** Serves build/ on a free local port. Returns the site's URL and a close function. */
export async function serveBuild(root) {
  const build = path.join(root, 'build');
  const server = http.createServer((req, res) => {
    const url = decodeURIComponent(new URL(req.url, 'http://x').pathname).replace(/^\/zero-to-prod/, '');
    const base = path.join(build, path.normalize(url));
    const file = [base, base + '.html', path.join(base, 'index.html')].find((f) => f.startsWith(build) && fs.existsSync(f) && fs.statSync(f).isFile());
    if (!file) return res.writeHead(404).end();
    res.writeHead(200, {'Content-Type': types[path.extname(file)] ?? 'application/octet-stream'}).end(fs.readFileSync(file));
  });
  await new Promise((r) => server.listen(0, '127.0.0.1', r));
  return {origin: `http://127.0.0.1:${server.address().port}/zero-to-prod/`, close: () => server.close()};
}
