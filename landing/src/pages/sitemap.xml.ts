// Sitemap (feature: growth). ponytail: hand-rolled instead of @astrojs/sitemap; same output, no new dep.
import { SITE, growthPages } from '../data/growth';

const docs = Object.keys(import.meta.glob('./docs/*.astro')).map((f) => {
  const name = f.replace('./docs/', '').replace('.astro', '');
  return name === 'index' ? '/docs' : `/docs/${name}`;
});

const paths = ['/', ...docs, ...growthPages.map((p) => `/${p.section}/${p.slug}`), '/tools/keyword-tester'];

export function GET() {
  const urls = paths.map((p) => `  <url><loc>${SITE}${p}</loc></url>`).join('\n');
  const body = `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${urls}\n</urlset>\n`;
  return new Response(body, { headers: { 'Content-Type': 'application/xml' } });
}
