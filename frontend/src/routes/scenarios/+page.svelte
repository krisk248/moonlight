<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, type ScenarioSummary } from '$lib/api';

	let scenarios = $state<ScenarioSummary[]>([]);
	let error = $state('');
	let actingOn = $state('');
	let showForm = $state(false);
	let newName = $state('');
	let newUrl = $state('');
	let submitting = $state(false);
	let timer: ReturnType<typeof setInterval> | null = null;

	async function refresh() {
		try {
			scenarios = await api.listScenarios();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function trigger(name: string, kind: 'baseline' | 'run') {
		actingOn = `${name}:${kind}`;
		try {
			const job = kind === 'baseline' ? await api.baseline(name) : await api.run(name);
			goto(`/jobs/${job.id}`);
		} catch (e) {
			error = (e as Error).message;
			actingOn = '';
		}
	}

	async function del(name: string) {
		if (!confirm(`Delete scenario "${name}" and its baseline?`)) return;
		try {
			await api.deleteScenario(name);
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function createEmpty() {
		submitting = true;
		try {
			await api.createScenario(newName, newUrl);
			showForm = false;
			newName = '';
			newUrl = '';
			await refresh();
		} catch (e) {
			error = (e as Error).message;
		} finally {
			submitting = false;
		}
	}

	async function recordFlow() {
		submitting = true;
		try {
			const job = await api.recordScenario(newName, newUrl);
			showForm = false;
			newName = '';
			newUrl = '';
			goto(`/jobs/${job.id}`);
		} catch (e) {
			error = (e as Error).message;
			submitting = false;
		}
	}

	onMount(() => {
		refresh();
		timer = setInterval(refresh, 5000);
	});
	onDestroy(() => {
		if (timer) clearInterval(timer);
	});
</script>

{#if error}
	<div class="card" style="border-color: var(--fail);">
		<strong>Error:</strong>
		{error}
		<button class="btn small" style="float: right" onclick={() => (error = '')}>dismiss</button>
	</div>
{/if}

<section class="card">
	<header>
		<h2>Scenarios</h2>
		<button class="btn primary" onclick={() => (showForm = !showForm)}>
			{showForm ? 'Cancel' : '+ New Scenario'}
		</button>
	</header>

	{#if showForm}
		<div class="form-block">
			<div class="form-row">
				<label>
					<span>Name</span>
					<input
						type="text"
						bind:value={newName}
						placeholder="login-flow"
						pattern="[a-zA-Z0-9_-]+"
						title="letters, digits, dash, underscore"
					/>
				</label>
				<label>
					<span>Start URL</span>
					<input type="url" bind:value={newUrl} placeholder="https://your-app.example.com" />
				</label>
			</div>
			<div class="form-actions">
				<button class="btn primary" disabled={submitting || !newName || !newUrl} onclick={recordFlow}>
					{submitting ? '…' : '🎥 Record in browser'}
				</button>
				<button class="btn" disabled={submitting || !newName || !newUrl} onclick={createEmpty}>
					Create empty (write YAML myself)
				</button>
			</div>
			<p class="muted" style="font-size: 12px; margin-top: 12px;">
				<strong>Record</strong> opens a Chromium window on your desktop via
				<code>npx playwright codegen</code>. Click through your flow, then close
				the window — Moonlight saves it as a YAML scenario. (Requires Node.js +
				npm installed locally.)
			</p>
		</div>
	{/if}

	{#if scenarios.length > 0}
		<table class="table">
			<thead>
				<tr>
					<th>Name</th>
					<th>URL</th>
					<th>Steps</th>
					<th>Baseline</th>
					<th>Login</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				{#each scenarios as s (s.name)}
					<tr>
						<td><a href="/scenarios/{s.name}"><code>{s.name}</code></a></td>
						<td class="truncate">{s.url}</td>
						<td>{s.steps}</td>
						<td>{s.has_baseline ? '✓' : '—'}</td>
						<td>{s.has_storage ? '🔒' : '—'}</td>
						<td class="actions">
							{#if !s.has_baseline}
								<button
									class="btn small"
									disabled={actingOn === `${s.name}:baseline`}
									onclick={() => trigger(s.name, 'baseline')}
								>
									{actingOn === `${s.name}:baseline` ? '…' : 'Baseline'}
								</button>
							{/if}
							<button
								class="btn small primary"
								disabled={!s.has_baseline || actingOn === `${s.name}:run`}
								onclick={() => trigger(s.name, 'run')}
							>
								{actingOn === `${s.name}:run` ? '…' : 'Run'}
							</button>
							<button
								class="btn small"
								style="color: var(--fail-fg);"
								onclick={() => del(s.name)}
								title="Delete this scenario"
							>
								Delete
							</button>
						</td>
					</tr>
				{/each}
			</tbody>
		</table>
	{:else}
		<p class="empty">No scenarios yet. Click <strong>+ New Scenario</strong> to create one.</p>
	{/if}
</section>

<style>
	.form-block {
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		padding: 18px 20px;
		margin-bottom: 18px;
	}
	.form-row {
		display: grid;
		grid-template-columns: 1fr 2fr;
		gap: 14px;
		margin-bottom: 12px;
	}
	.form-row label {
		display: flex;
		flex-direction: column;
		gap: 4px;
		font-size: 13px;
		color: var(--muted);
	}
	.form-row input {
		background: var(--bg);
		color: var(--text);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 8px 12px;
		font-size: 14px;
		font-family: inherit;
	}
	.form-actions {
		display: flex;
		gap: 10px;
	}
</style>
