import { index, route, type RouteConfig } from '@react-router/dev/routes';

export default [
  index('routes/landing.tsx'),
  route('legal/privacy', 'routes/legal/privacy.tsx'),
  route('legal/terms', 'routes/legal/terms.tsx'),
  route('api/search', 'routes/search.ts'),
  route('og/docs/*', 'routes/og.docs.tsx'),

  // LLM integration:
  route('llms.txt', 'llms/index.ts'),
  route('llms-full.txt', 'llms/full.ts'),
  route('llms.mdx/*', 'llms/mdx.ts'),
  route('docs/*', 'routes/docs.tsx'),
] satisfies RouteConfig;
