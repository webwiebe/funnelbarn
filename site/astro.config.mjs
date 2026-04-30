import { defineConfig } from 'astro/config';
import tailwind from '@astrojs/tailwind';

export default defineConfig({
  site: 'https://funnelbarn.webwiebe.nl',
  integrations: [tailwind()],
  output: 'static',
});
