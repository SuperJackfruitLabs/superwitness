// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import starlightLlmsTxt from 'starlight-llms-txt';

export default defineConfig({
  site: 'https://docs.superwitness.dev',
  integrations: [
    starlight({
      title: 'superwitness',
      description:
        'Observability and evaluation for agent fleets: what your agents did, how your products behaved, and whether the work was any good, joined on one run.',
      // /llms.txt, /llms-full.txt and /llms-small.txt, for agents reading these docs. The
      // full file carries every page, reference pages included; the small one drops asides
      // and <details> but keeps every page too, since nothing here is noise.
      plugins: [
        starlightLlmsTxt({
          projectName: 'superwitness',
          description:
            'superwitness is observability and evaluation for agent fleets: what your agents did, how ' +
            'your products behaved while they did it, and whether the work was any good, joined on ' +
            'one run. A run document answers three questions: what did it do (the attempts, the agent ' +
            'configuration behind each, and their trace spans), how did the products run (the logs ' +
            "and errors that share the run's id or trace), and was it any good (gate decisions read " +
            'from superpipeline and verdicts recorded in superwitness). It is a single Go binary and ' +
            'a Postgres database, self-hosted and MIT licensed, with an HTTP API and MCP tools for ' +
            'building on it.',
          optionalLinks: [
            { label: 'superwitness', url: 'https://superwitness.dev', description: 'The product site.' },
            { label: 'Source', url: 'https://github.com/SuperJackfruitLabs/superwitness', description: 'The superwitness repository on GitHub.' },
          ],
        }),
      ],
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
