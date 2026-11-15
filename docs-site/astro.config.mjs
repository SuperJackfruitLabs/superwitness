// @ts-check
import { defineConfig } from 'astro/config';
import starlight from '@astrojs/starlight';
import sjlDocs from '@superjackfruitlabs/vibekit-docs';

export default defineConfig({
  site: 'https://docs.superwitness.dev',
  integrations: [
    starlight({
      title: 'SuperWitness',
      description:
        'Observability and evaluation for agent fleets: what your agents did, how your products behaved, and whether the work was any good, joined on one run.',
      // The suite's docs theme: vibekit's jade, the faces, the street, status badges, the footers,
      // the 404, the edit link and llms.txt. See @superjackfruitlabs/vibekit-docs's README.
      plugins: [
        sjlDocs({
          product: 'superwitness',
          status: 'half-lit',
          // /llms.txt, /llms-full.txt and /llms-small.txt, for agents reading these docs.
          llms: {
            description:
              'SuperWitness is observability and evaluation for agent fleets: what your agents did, how ' +
              'your products behaved while they did it, and whether the work was any good, joined on ' +
              'one run. A run document answers three questions: what did it do (the attempts, the agent ' +
              'configuration behind each, and their trace spans), how did the products run (the logs ' +
              "and errors that share the run's id or trace), and was it any good (gate decisions read " +
              'from SuperPipeline and verdicts recorded in SuperWitness). It is a single Go binary and ' +
              'a Postgres database, self-hosted and MIT licensed, with an HTTP API and MCP tools for ' +
              'building on it.',
          },
        }),
      ],
      favicon: '/favicon.svg',
      head: [
        // The link preview, copied from landing/public/og.png so this host serves its own.
        // Absolute URLs: several unfurlers ignore a relative og:image.
        { tag: 'meta', attrs: { property: 'og:image', content: 'https://docs.superwitness.dev/og.png' } },
        { tag: 'meta', attrs: { property: 'og:image:width', content: '1200' } },
        { tag: 'meta', attrs: { property: 'og:image:height', content: '630' } },
        { tag: 'meta', attrs: { property: 'og:image:alt', content: 'SuperWitness — every run, on the record' } },
        { tag: 'meta', attrs: { name: 'twitter:image', content: 'https://docs.superwitness.dev/og.png' } },
      ],
      social: [{ icon: 'github', label: 'GitHub', href: 'https://github.com/SuperJackfruitLabs/superwitness' }],
      // The suite's five groups, in the order a reader needs them.
      sidebar: [
        {
          label: 'Start',
          items: [
            { label: 'What SuperWitness is', slug: 'what-it-is' },
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
          ],
        },
        {
          label: 'Build on it',
          items: [
            { label: 'Sending telemetry', slug: 'use/telemetry' },
            { label: 'With AgentPod and SuperPipeline', slug: 'use/with-agentpod-and-superpipeline' },
            { label: 'HTTP API', slug: 'build/api' },
            { label: 'Run registry API', slug: 'build/run-registry' },
            { label: 'MCP tools', slug: 'build/mcp' },
          ],
        },
        {
          label: 'Run it',
          items: [
            { label: 'Operations', slug: 'use/operations' },
            { label: 'The canary', slug: 'use/canary' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'Configuration', slug: 'use/configuration' },
            { label: 'Contracts', slug: 'build/contracts' },
            { label: 'Licences', slug: 'build/licences' },
          ],
        },
      ],
    }),
  ],
});
