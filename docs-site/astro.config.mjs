// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';

export default defineConfig({
  site: 'https://docs.superwitness.dev',
  integrations: [
    starlight({
      title: 'superwitness',
      description:
        'Observability and evaluation for agent fleets: what your agents did, how your products behaved, and whether the work was any good, joined on one run.',
      // The mark beside the name, as on the landing page. See the component for why it
      // replaces Starlight's own rather than using the `logo` option.
      components: { SiteTitle: './src/components/SiteTitle.astro' },
      // The landing page's palette and faces; see src/styles/theme.css for each value.
      customCss: ['./src/styles/theme.css'],
      favicon: '/favicon.svg',
      head: [
        { tag: 'link', attrs: { rel: 'preconnect', href: 'https://fonts.googleapis.com' } },
        { tag: 'link', attrs: { rel: 'preconnect', href: 'https://fonts.gstatic.com', crossorigin: true } },
        {
          tag: 'link',
          attrs: {
            rel: 'stylesheet',
            href:
              'https://fonts.googleapis.com/css2?family=Fraunces:opsz,wght@9..144,600' +
              '&family=IBM+Plex+Mono:wght@400;500&family=IBM+Plex+Sans:wght@400;500;600&display=swap',
          },
        },
        // The link preview, copied from landing/public/og.png so this host serves its own.
        // Absolute URLs: several unfurlers ignore a relative og:image.
        { tag: 'meta', attrs: { property: 'og:image', content: 'https://docs.superwitness.dev/og.png' } },
        { tag: 'meta', attrs: { property: 'og:image:width', content: '1200' } },
        { tag: 'meta', attrs: { property: 'og:image:height', content: '630' } },
        { tag: 'meta', attrs: { property: 'og:image:alt', content: 'superwitness — every run, on the record' } },
        { tag: 'meta', attrs: { name: 'twitter:image', content: 'https://docs.superwitness.dev/og.png' } },
      ],
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/SuperJackfruitLabs/superwitness' }],
      // Three sections, in the order a reader needs them: what it is and how to stand it
      // up, then using it, then building on it.
      sidebar: [
        {
          label: 'Start',
          items: [
            { label: 'What superwitness is', slug: 'what-it-is' },
            { label: 'Install', slug: 'install' },
            { label: 'Concepts', slug: 'concepts' },
          ],
        },
        {
          label: 'Use it',
          items: [
            { label: 'The app', slug: 'use/the-app' },
            { label: 'Read a run', slug: 'use/read-a-run' },
            { label: 'Transcripts', slug: 'use/transcripts' },
            { label: 'Verdicts', slug: 'use/verdicts' },
            { label: 'Sending telemetry', slug: 'use/telemetry' },
            { label: 'With AgentPod and superpipeline', slug: 'use/with-agentpod-and-superpipeline' },
            { label: 'Operations', slug: 'use/operations' },
            { label: 'Configuration', slug: 'use/configuration' },
          ],
        },
        {
          label: 'Build on it',
          items: [
            { label: 'HTTP API', slug: 'build/api' },
            { label: 'Run registry API', slug: 'build/run-registry' },
            { label: 'MCP tools', slug: 'build/mcp' },
            { label: 'Contracts', slug: 'build/contracts' },
            { label: 'Licences', slug: 'build/licences' },
          ],
        },
      ],
    }),
  ],
});
