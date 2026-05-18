<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, type RunSummary } from '$lib/api';

	let runs = $state<RunSummary[]>([]);
	let error = $state('');
	let timer: ReturnType<typeof setInterval> | null = null;

	async function refresh() {
		try {
			runs = await api.listRuns();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	const grouped = $derived.by(() => {
		const m = new Map<string, RunSummary[]>();
		for (const r of runs) {
			if (!m.has(r.scenario)) m.set(r.scenario, []);
			m.get(r.scenario)!.push(r);
		}
		return [...m.entries()];
	});

	const stats = $derived({
		total: runs.length,
		passed: runs.filter((r) => r.passed).length,
		failed: runs.filter((r) => !r.passed).length
	});

	onMount(() => {
		refresh();
		timer = setInterval(refresh, 4000);
	});
	onDestroy(() => {
		if (timer) clearInterval(timer);
	});
</script>

{#if error}
	<div class="card" style="border-color: var(--fail);">
		<strong>Error:</strong>
		{error}
	</div>
{/if}

<section class="card">
	<header>
		<h2>Run history</h2>
		<div style="display: flex; gap: 12px; font-size: 13px;">
			<span class="muted">{stats.total} runs</span>
			<span class="badge pass">{stats.passed} pass</span>
			<span class="badge fail">{stats.failed} fail</span>
		</div>
	</header>

	{#if runs.length > 0}
		{#each grouped as [scenarioName, scenarioRuns]}
			<h3 style="margin-top: 24px;"><code>{scenarioName}</code> · {scenarioRuns.length} run(s)</h3>
			<table class="table">
				<thead>
					<tr>
						<th>Verdict</th>
						<th>Started</th>
						<th>Steps</th>
						<th></th>
					</tr>
				</thead>
				<tbody>
					{#each scenarioRuns as r (r.id)}
						<tr>
							<td>
								<span class="badge {r.passed ? 'pass' : 'fail'}">
									{r.passed ? 'PASS' : 'FAIL'}
								</span>
							</td>
							<td class="muted">{r.started_at}</td>
							<td>{r.step_count} ({r.fail_count} failed)</td>
							<td><a class="btn small" href="/runs/{r.id}">View report</a></td>
						</tr>
					{/each}
				</tbody>
			</table>
		{/each}
	{:else}
		<p class="empty">No runs yet. Trigger a scenario from the Scenarios tab to see history here.</p>
	{/if}
</section>
