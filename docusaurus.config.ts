import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

// This runs in Node.js — don't use browser APIs or JSX here.

const GITHUB_USER = 'jawwadzafar';
const REPO = 'zero-to-prod';

const config: Config = {
  title: 'Zero to Prod',
  tagline:
    'From basic tech literacy to working backend, DevOps and AI engineer — one rung at a time, by building and shipping one real system. Free, forever.',
  favicon: 'img/favicon.svg',

  future: {
    v4: true,
    faster: true,
  },

  url: `https://${GITHUB_USER}.github.io`,
  baseUrl: `/${REPO}/`,
  organizationName: GITHUB_USER,
  projectName: REPO,
  trailingSlash: false,

  onBrokenLinks: 'throw',
  onBrokenAnchors: 'warn',

  markdown: {
    mermaid: true,
    hooks: {onBrokenMarkdownLinks: 'throw'},
  },

  i18n: {defaultLocale: 'en', locales: ['en']},

  presets: [
    [
      'classic',
      {
        docs: {
          routeBasePath: 'learn',
          sidebarPath: './sidebars.ts',
          editUrl: `https://github.com/${GITHUB_USER}/${REPO}/edit/main/`,
          breadcrumbs: true,
        },
        blog: false,
        theme: {customCss: './src/css/custom.css'},
        sitemap: {changefreq: 'weekly'},
      } satisfies Preset.Options,
    ],
  ],

  themes: [
    '@docusaurus/theme-mermaid',
    [
      '@easyops-cn/docusaurus-search-local',
      {
        hashed: true,
        docsRouteBasePath: 'learn',
        indexBlog: false,
        highlightSearchTermsOnTargetPage: true,
        explicitSearchResultPath: true,
      },
    ],
  ],

  plugins: ['./plugins/curriculum.js'],

  themeConfig: {
    image: 'img/social-card.png',
    colorMode: {respectPrefersColorScheme: true},
    docs: {sidebar: {hideable: true, autoCollapseCategories: true}},
    announcementBar: {
      id: 'free-forever',
      content:
        '100% free and open. No sign-up — your progress is saved in your browser. ⭐ <a target="_blank" rel="noopener" href="https://github.com/jawwadzafar/zero-to-prod">Star it on GitHub</a> if it helps you.',
      isCloseable: true,
    },
    navbar: {
      title: 'Zero to Prod',
      logo: {alt: 'Zero to Prod', src: 'img/logo.svg'},
      items: [
        {type: 'docSidebar', sidebarId: 'learn', position: 'left', label: 'Learn'},
        {to: '/roadmap', label: 'Roadmap', position: 'left'},
        {to: '/progress', label: 'My progress', position: 'left'},
        {href: `https://github.com/${GITHUB_USER}/${REPO}`, label: 'GitHub', position: 'right'},
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Learn',
          items: [
            {label: 'Start here', to: '/learn'},
            {label: 'Roadmap', to: '/roadmap'},
            {label: 'My progress', to: '/progress'},
            {label: 'Glossary', to: '/learn/glossary'},
          ],
        },
        {
          title: 'Build',
          items: [
            {label: 'The snip project', href: `https://github.com/${GITHUB_USER}/${REPO}/tree/main/snip`},
            {label: 'AI examples', href: `https://github.com/${GITHUB_USER}/${REPO}/tree/main/ai`},
          ],
        },
        {
          title: 'Contribute',
          items: [
            {label: 'GitHub', href: `https://github.com/${GITHUB_USER}/${REPO}`},
            {label: 'Report a mistake', href: `https://github.com/${GITHUB_USER}/${REPO}/issues/new`},
            {label: 'How we write', href: `https://github.com/${GITHUB_USER}/${REPO}/blob/main/STYLE.md`},
          ],
        },
      ],
      copyright: `Zero to Prod — free to read, forever. Text CC BY-SA 4.0 · Code MIT.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'go', 'sql', 'yaml', 'json', 'docker', 'hcl', 'nginx', 'python', 'toml', 'ini', 'diff', 'protobuf'],
    },
    mermaid: {theme: {light: 'neutral', dark: 'dark'}},
  } satisfies Preset.ThemeConfig,
};

export default config;
