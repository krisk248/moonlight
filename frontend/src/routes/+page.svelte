<script lang="ts">
	import { onMount, onDestroy } from 'svelte';
	import { api, type StatusResponse, type ScenarioSummary, type RunSummary } from '$lib/api';

	let status = $state<StatusResponse | null>(null);
	let scenarios = $state<ScenarioSummary[]>([]);
	let runs = $state<RunSummary[]>([]);
	let error = $state('');
	let timer: ReturnType<typeof setInterval> | null = null;

	async function refresh() {
		try {
			[status, scenarios, runs] = await Promise.all([
				api.status(),
				api.listScenarios(),
				api.listRuns()
			]);
		} catch (e) {
			error = (e as Error).message;
		}
	}

	const stats = $derived({
		total: runs.length,
		passed: runs.filter((r) => r.passed).length,
		failed: runs.filter((r) => !r.passed).length,
		scenarios: scenarios.length,
		withBaseline: scenarios.filter((s) => s.has_baseline).length,
		aiScenarios: scenarios.filter((s) => s.needs_ai).length,
		basicScenarios: scenarios.filter((s) => !s.needs_ai).length
	});

	const latest = $derived(runs.slice(0, 5));

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
	</div>
{/if}

<section class="card">
	<header>
		<h2>System</h2>
	</header>
	{#if status}
		<div class="grid-3">
			<div class="status-pill ok">
				<span class="label">Version</span>
				<span class="value">{status.version}</span>
			</div>
			<div class="status-pill {status.is_source_build ? 'fail' : 'ok'}">
				<span class="label">Build</span>
				<span class="value">{status.build_id}</span>
			</div>
			<div class="status-pill {status.revocation_active ? 'ok' : 'fail'}">
				<span class="label">Kill switch</span>
				<span class="value">
					{status.revocation_active ? 'enforcing' : 'inactive (source build)'}
				</span>
			</div>
		</div>
		{#if status.ai_enabled}
			<div class="grid-3" style="margin-top: 12px;">
				<div class="status-pill {status.ollama_up ? 'ok' : 'fail'}">
					<span class="label">Ollama server</span>
					<span class="value">{status.ollama_up ? 'reachable' : 'unreachable'}</span>
				</div>
				<div class="status-pill {status.ollama_model_present ? 'ok' : 'fail'}">
					<span class="label">Vision model</span>
					<span class="value">
						{status.ollama_model}<br />
						<span class="muted" style="font-size: 11px;">
							{status.ollama_model_present
								? 'pulled & ready'
								: status.ollama_up
									? 'NOT pulled'
									: 'Ollama unreachable'}
						</span>
					</span>
				</div>
				<div class="status-pill ok">
					<span class="label">Mode</span>
					<span class="value">AI-augmented (toggle off in Settings)</span>
				</div>
			</div>
		{/if}
	{:else}
		<p class="muted">Loading…</p>
	{/if}
</section>

<section class="card">
	<header>
		<h2>At a glance</h2>
	</header>
	<div class="grid-3">
		<div class="status-pill ok">
			<span class="label">Scenarios</span>
			<span class="value">
				{stats.scenarios} total ({stats.withBaseline} with baseline)
				{#if status?.ai_enabled}
					<br /><span class="muted" style="font-size: 11px;">
						🤖 {stats.aiScenarios} AI · 🔍 {stats.basicScenarios} basic
					</span>
				{/if}
			</span>
		</div>
		<div class="status-pill {stats.failed > 0 ? 'fail' : 'ok'}">
			<span class="label">Recent runs</span>
			<span class="value">
				{stats.total} total · <span style="color: var(--pass-fg)">{stats.passed} pass</span>
				· <span style="color: var(--fail-fg)">{stats.failed} fail</span>
			</span>
		</div>
		<div class="status-pill ok">
			<span class="label">Shortcuts</span>
			<span class="value">
				<a href="/scenarios">Manage scenarios →</a><br />
				<a href="/history">Run history →</a>
			</span>
		</div>
	</div>
</section>

<section class="card">
	<header>
		<h2>Latest runs</h2>
		<a href="/history" class="btn small">See all</a>
	</header>
	{#if latest.length > 0}
		<table class="table">
			<thead>
				<tr>
					<th>Verdict</th>
					<th>Scenario</th>
					<th>Started</th>
					<th>Steps</th>
					<th></th>
				</tr>
			</thead>
			<tbody>
				{#each latest as r (r.id)}
					<tr>
						<td>
							<span class="badge {r.passed ? 'pass' : 'fail'}">
								{r.passed ? 'PASS' : 'FAIL'}
							</span>
						</td>
						<td><code>{r.scenario}</code></td>
						<td class="muted">{r.started_at}</td>
						<td>{r.step_count} ({r.fail_count} failed)</td>
						<td><a class="btn small" href="/runs/{r.id}">View report</a></td>
					</tr>
				{/each}
			</tbody>
		</table>
	{:else}
		<p class="empty">No runs yet. Head to <a href="/scenarios">Scenarios</a> to create or run one.</p>
	{/if}
</section>
