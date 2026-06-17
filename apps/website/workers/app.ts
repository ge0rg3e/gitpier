import { createRequestHandler } from '@react-router/cloudflare';

const requestHandler = createRequestHandler(
  () => import('virtual:react-router/server-build'),
);

export default {
  async fetch(request, env, ctx) {
    return requestHandler(request, {
      cloudflare: { env, ctx },
    });
  },
} satisfies ExportedHandler<CloudflareEnvironment>;
