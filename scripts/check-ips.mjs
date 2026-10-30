#!/usr/bin/env node
// Finds IPv4 addresses that could belong to someone real. CLAUDE.md's rule:
// examples use placeholders (203.0.113.10 and the other documentation ranges),
// never a real server's address. Private, loopback, documentation, reserved
// and a few well-known public resolvers are fine; anything else fails.
// Spec section numbers ("section 3.1.3.7") are skipped. Any other line that
// genuinely needs one can say so with the comment "ip-check: ignore".
//
//   node scripts/check-ips.mjs
import {execFileSync} from 'node:child_process';
import fs from 'node:fs';

const allowed = [
  /^0\./, /^10\./, /^127\./, /^169\.254\./, /^172\.(1[6-9]|2\d|3[01])\./, /^192\.168\./, // private, loopback, link-local
  /^100\.(6[4-9]|[7-9]\d|1[01]\d|12[0-7])\./, // carrier-grade NAT
  /^192\.0\.2\./, /^198\.51\.100\./, /^203\.0\.113\./, // documentation (RFC 5737)
  /^192\.0\.0\./, /^198\.1[89]\./, /^2(2[4-9]|[3-5]\d)\./, // reserved, benchmarking, multicast and above
  /^255\./, // netmasks
  /^(1\.1\.1\.1|1\.0\.0\.1|8\.8\.8\.8|8\.8\.4\.4|9\.9\.9\.9)$/, // public resolvers the docs name on purpose
];

const files = execFileSync('git', ['ls-files'], {encoding: 'utf8'})
  .split('\n')
  .filter((f) => /\.(mdx?|tsx?|m?js|go|py|ya?ml|json|tf|sql|conf|toml|sh|txt)$|Dockerfile$/.test(f))
  .filter((f) => !/(^|\/)(package-lock\.json|go\.sum|uv\.lock)$/.test(f) && fs.existsSync(f));

const bad = [];
for (const file of files) {
  fs.readFileSync(file, 'utf8').split('\n').forEach((line, i) => {
    if (line.includes('ip-check: ignore')) return;
    // Four dotted numbers, not part of a longer dotted run (a version like 1.2.3.4.5).
    for (const [ip] of line.matchAll(/(?<![\d.])(?<!section )(?:\d{1,3}\.){3}\d{1,3}(?![\d.]*\d)/g)) {
      if (ip.split('.').some((n) => Number(n) > 255)) continue; // not an address
      if (!allowed.some((r) => r.test(ip))) bad.push(`${file}:${i + 1}: ${ip}`);
    }
  });
}

if (bad.length) {
  console.error(`Addresses that may be real (use 203.0.113.x, 198.51.100.x or 192.0.2.x):\n${bad.join('\n')}`);
  process.exit(1);
}
console.log(`No real-looking IP addresses in ${files.length} files.`);
