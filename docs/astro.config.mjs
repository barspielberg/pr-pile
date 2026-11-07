import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

// Phase 1 sidebar lists only the pages that exist. Starlight fails the build
// on an unknown slug, so the rest of the §5.2 sitemap joins the sidebar as each
// page lands: Guide, Reference, Design, Help, Contributing, and
// getting-started/board-tour.
export default defineConfig({
  site: 'https://barspielberg.github.io',
  base: '/pr-pile',
  integrations: [
    starlight({
      title: 'pile',
      description: 'A terminal PR board for GitHub.',
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/barspielberg/pr-pile' }],
      editLink: { baseUrl: 'https://github.com/barspielberg/pr-pile/edit/main/docs/' },
      sidebar: [
        {
          label: 'Getting started',
          items: ['getting-started/install', 'getting-started/first-run'],
        },
        {
          label: 'Guide',
          items: [
            'guide/rules',
            'guide/reading-the-board',
            'guide/search',
            'guide/detail',
            {
              label: 'Actions',
              items: ['guide/actions', 'guide/actions/worktree', 'guide/actions/review-tui'],
            },
            'guide/team-board',
          ],
        },
        {
          label: 'Reference',
          items: ['reference/config', 'reference/rules', 'reference/actions', 'reference/keys', 'reference/cli'],
        },
      ],
    }),
  ],
});
