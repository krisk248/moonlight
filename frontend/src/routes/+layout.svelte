<script lang="ts">
	import '../app.css';
	import { page } from '$app/state';
	let { children } = $props();

	const tabs = [
		{ href: '/', label: 'Dashboard' },
		{ href: '/scenarios', label: 'Scenarios' },
		{ href: '/suites', label: 'Suites' },
		{ href: '/history', label: 'History' },
		{ href: '/settings', label: 'Settings' }
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
	Made by <strong>Kannan</strong>
</footer>

<style>
	.nav a.active {
		background: var(--surface-hover);
		color: var(--accent);
	}
</style>
