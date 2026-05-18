<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, type Scenario } from '$lib/api';

	let name = $derived(page.params.name);
	let scenario = $state<Scenario | null>(null);
	let error = $state('');
	let acting = $state(false);

	async function load() {
		try {
			scenario = await api.getScenario(name);
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function trigger(kind: 'baseline' | 'run') {
		acting = true;
		try {
			const job = kind === 'baseline' ? await api.baseline(name) : await api.run(name);
			goto(`/jobs/${job.id}`);
		} catch (e) {
			error = (e as Error).message;
			acting = false;
		}
	}

	async function del() {
		if (!confirm(`Delete scenario "${name}" and its baseline?`)) return;
		await api.deleteScenario(name);
		goto('/');
	}

	$effect(() => {
		if (name) load();
	});
</script>

{#if error}
	<div class="card" style="border-color: var(--fail);">
		<strong>Error:</strong>
		{error}
	</div>
{/if}

{#if scenario}
	<section class="card">
		<header>
			<div>
				<h2><code>{scenario.name}</code></h2>
				<p class="muted" style="margin: 4px 0 0; font-size: 13px;">
					{scenario.url} · {scenario.steps.length} steps
					{#if scenario.storage_state}· 🔒 has login state{/if}
				</p>
			</div>
			<div style="display: flex; gap: 8px;">
				<button class="btn" disabled={acting} onclick={() => trigger('baseline')}>
					Capture Baseline
				</button>
				<button class="btn primary" disabled={acting} onclick={() => trigger('run')}>
					Run Regression
				</button>
				<button class="btn" onclick={del} style="color: var(--fail-fg)">Delete</button>
			</div>
		</header>

		<h3>Steps</h3>
		<ol style="padding-left: 22px;">
			{#each scenario.steps as step, i}
				<li style="padding: 6px 0;">
					<code>{step.action}</code>
					{#if step.url}<span class="muted"> → {step.url}</span>{/if}
					{#if step.selector}<span class="muted"> <code>{step.selector}</code></span>{/if}
					{#if step.role}<span class="muted"> role=<code>{step.role}</code>{step.role_name ? ` name="${step.role_name}"` : ''}</span>{/if}
					{#if step.label}<span class="muted"> label=<code>{step.label}</code></span>{/if}
					{#if step.text}<span class="muted"> text=<code>{step.text}</code></span>{/if}
					{#if step.value}<span class="muted"> = "{step.value.slice(0, 40)}"</span>{/if}
					{#if step.key}<span class="muted"> key={step.key}</span>{/if}
					{#if step.name && step.action === 'screenshot'}<span class="muted"> ({step.name})</span>{/if}
					{#if step.ai_check}
						<div style="margin-top: 6px; padding-left: 16px; color: var(--muted); font-size: 13px;">
							🤖 <i>{step.ai_check.prompt}</i> → expect <strong>{step.ai_check.expect}</strong>
						</div>
					{/if}
					{#if step.dom_check}
						<div style="margin-top: 6px; padding-left: 16px; color: var(--muted); font-size: 13px;">
							🔍 dom_check:
							{#if step.dom_check.contains_text}contains_text={JSON.stringify(step.dom_check.contains_text)}{/if}
							{#if step.dom_check.selector_visible} selector_visible=<code>{step.dom_check.selector_visible}</code>{/if}
							{#if step.dom_check.url_contains} url_contains=<code>{step.dom_check.url_contains}</code>{/if}
						</div>
					{/if}
				</li>
			{/each}
		</ol>
	</section>
{:else if !error}
	<p class="muted">Loading…</p>
{/if}
