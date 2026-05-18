<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	let { children } = $props();

	const tabs = [
		{ href: '/', label: 'Dashboard' },
		{ href: '/scenarios', label: 'Scenarios' },
		{ href: '/history', label: 'History' }
	];
	let active = $derived(page.url.pathname);
</script>

<nav class="nav">
	<a href="/" class="brand">🌙 Moonlight</a>
	<span class="brand-sub">automated regression testing</span>
	<span class="spacer"></span>
	{#each tabs as t}
		<a
			href={t.href}
			class:active={active === t.href || (t.href !== '/' && active.startsWith(t.href))}
		>
			{t.label}
		</a>
	{/each}
</nav>

<main class="container">
	{@render children()}
</main>

<footer class="site-footer">
	Made by <strong>Kannan</strong> ·
	<a href="https://github.com/krisk248/moonlight" target="_blank" rel="noopener">
		github.com/krisk248/moonlight
	</a>
</footer>

<style>
	.nav a.active {
		background: var(--surface-hover);
		color: var(--accent);
	}
</style>
