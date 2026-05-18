<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, type ScenarioSummary } from '$lib/api';

	let scenarios = $state<ScenarioSummary[]>([]);
	let error = $state('');
	let running = $state<Record<string, boolean>>({});

	async function load() {
		try {
			scenarios = await api.listScenarios();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	// Group scenarios by tag. Scenarios with no tags go under "untagged".
	const grouped = $derived.by(() => {
		const map = new Map<string, ScenarioSummary[]>();
		for (const s of scenarios) {
			const tags = s.tags && s.tags.length > 0 ? s.tags : ['untagged'];
			for (const t of tags) {
				if (!map.has(t)) map.set(t, []);
				map.get(t)!.push(s);
			}
		}
		return [...map.entries()].sort((a, b) => a[0].localeCompare(b[0]));
	});

	async function runSuite(tag: string, list: ScenarioSummary[]) {
		running[tag] = true;
		try {
			// Fire baselines for scenarios that have none, then run all in series.
			for (const s of list) {
				if (!s.has_baseline) {
					await api.baseline(s.name);
				}
			}
			// Kick off the first run; the user can watch jobs in the history page.
			if (list.length > 0) {
				const job = await api.run(list[0].name);
				goto(`/jobs/${job.id}`);
			}
		} catch (e) {
			error = (e as Error).message;
		} finally {
			running[tag] = false;
		}
	}

	onMount(load);
</script>

{#if error}
	<div class="card" style="border-color: var(--fail);">
		<strong>Error:</strong>
		{error}
	</div>
{/if}

<section class="card">
	<header>
		<h2>Suites</h2>
		<a href="/scenarios" class="btn small">Manage scenarios →</a>
	</header>
	<p class="muted" style="font-size: 13px; margin: 0 0 12px;">
		Group scenarios by tag. Add tags to a scenario YAML like
		<code>tags: [smoke, critical]</code>. Then run all members of a suite from here.
	</p>

	{#if grouped.length === 0}
		<p class="empty">No scenarios yet.</p>
	{:else}
		{#each grouped as [tag, list] (tag)}
			<details open style="margin-top: 16px;">
				<summary style="cursor: pointer; padding: 10px 12px; background: var(--bg); border-radius: 6px;">
					<strong style="font-size: 14px;">@{tag}</strong>
					<span class="muted" style="font-size: 12px; margin-left: 8px;">
						{list.length} scenario{list.length === 1 ? '' : 's'}
					</span>
					<button
						class="btn small primary"
						style="float: right;"
						disabled={running[tag]}
						onclick={(e) => {
							e.preventDefault();
							runSuite(tag, list);
						}}
					>
						{running[tag] ? 'Running…' : 'Run suite'}
					</button>
				</summary>
				<table class="table" style="margin-top: 8px;">
					<thead>
						<tr>
							<th>Name</th>
							<th>URL</th>
							<th>Steps</th>
							<th>Baseline</th>
						</tr>
					</thead>
					<tbody>
						{#each list as s (s.name)}
							<tr>
								<td><a href="/scenarios/{s.name}"><code>{s.name}</code></a></td>
								<td class="truncate">{s.url}</td>
								<td>{s.steps}</td>
								<td>{s.has_baseline ? '✓' : '—'}</td>
							</tr>
						{/each}
					</tbody>
				</table>
			</details>
		{/each}
	{/if}
</section>
