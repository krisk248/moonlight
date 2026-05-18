import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

/** @type {import('@sveltejs/kit').Config} */
const config = {
	preprocess: vitePreprocess(),
	compilerOptions: {
		runes: ({ filename }) => (filename.split(/[/\\]/).includes('node_modules') ? undefined : true)
	},
	kit: {
		// Output directly into the Go embed path. The Go binary picks up
		// whatever is in cmd/moonlight/web/ at compile time via go:embed.
		adapter: adapter({
			pages: '../cmd/moonlight/web',
			assets: '../cmd/moonlight/web',
			fallback: 'index.html', // SPA mode — client-side routing
			precompress: false,
			strict: true
		}),
		prerender: {
			handleHttpError: 'warn'
		}
	}
};

export default config;
