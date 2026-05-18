<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { api, type RunResult, fileURL } from '$lib/api';

	let id = $derived(page.params.id);
	let run = $state<RunResult | null>(null);
	let error = $state('');

	async function load() {
		try {
			run = await api.getRun(id);
		} catch (e) {
			error = (e as Error).message;
		}
	}

	$effect(() => {
		if (id) load();
	});

	function activityCount(s: import('$lib/api').StepResult): number {
		return (
			(s.console_messages?.length ?? 0) +
			(s.failed_requests?.length ?? 0) +
			(s.page_errors?.length ?? 0)
		);
	}
</script>

{#if error}
	<div class="card" style="border-color: var(--fail);">
		<strong>Error:</strong>
		{error}
	</div>
{/if}

{#if run}
	<section class="card">
		<header>
			<div>
				<h2>
					<code>{run.scenario}</code>
					<span class="badge {run.passed ? 'pass' : 'fail'}">
						{run.passed ? 'PASS' : 'FAIL'}
					</span>
					<span class="badge neutral">{run.mode}</span>
					{#if run.ai_used}<span class="badge neutral">AI used</span>{/if}
				</h2>
				<p class="muted" style="margin: 4px 0 0; font-size: 13px;">
					{run.url} · {run.started_at} → {run.finished_at} ·
					{run.steps.length} steps ({run.steps.filter((s) => !s.passed).length} failed)
				</p>
			</div>
			<a class="btn" href="/scenarios/{run.scenario}">Open scenario</a>
		</header>
	</section>

	{#if run.ai_summary}
		<section class="card ai-summary">
			<header><h2>AI summary</h2></header>
			<blockquote>{run.ai_summary}</blockquote>
		</section>
	{/if}

	{#each run.steps as s (s.index)}
		<article class="step-card {s.passed ? 'pass' : 'fail'}">
			<div class="step-card-head">
				<span class="step-num">#{s.index}</span>
				<span class="step-action">{s.action}</span>
				<span class="step-name">{s.name}</span>
				<span class="step-spacer"></span>
				<span class="badge {s.passed ? 'pass' : 'fail'}">
					{s.passed ? 'PASS' : 'FAIL'}
				</span>
			</div>

			{#if s.action === 'screenshot' && (s.screenshot || s.baseline)}
				<div class="images3">
					{#if s.baseline}
						<figure>
							<figcaption>Baseline</figcaption>
							<a href={fileURL(s.baseline)} target="_blank">
								<img src={fileURL(s.baseline)} loading="lazy" alt="baseline" />
							</a>
						</figure>
					{/if}
					{#if s.screenshot}
						<figure>
							<figcaption>Current</figcaption>
							<a href={fileURL(s.screenshot)} target="_blank">
								<img src={fileURL(s.screenshot)} loading="lazy" alt="current" />
							</a>
						</figure>
					{/if}
					{#if s.diff_overlay}
						<figure>
							<figcaption>
								Diff{#if s.diff_ratio !== undefined}
									({(s.diff_ratio * 100).toFixed(2)}% different){/if}
							</figcaption>
							<a href={fileURL(s.diff_overlay)} target="_blank">
								<img src={fileURL(s.diff_overlay)} loading="lazy" alt="diff" />
							</a>
						</figure>
					{/if}
				</div>
			{/if}

			{#if s.ai_narration}
				<div class="narration">
					<span class="narration-icon">AI sees:</span>
					<span class="narration-text">{s.ai_narration}</span>
				</div>
			{/if}

			{#if s.checks && s.checks.length > 0}
				{#each s.checks as c}
					<div class="verdict {c.passed ? 'pass' : 'fail'}">
						<div class="verdict-head">
							<span class="verdict-kind">
								{#if c.kind === 'ai_check'}AI looked at the screen
								{:else if c.kind === 'dom_check'}DOM assertions
								{:else}{c.kind}{/if}
							</span>
							<span class="badge small {c.passed ? 'pass' : 'fail'}">
								{c.passed ? 'PASS' : 'FAIL'}
							</span>
						</div>
						<blockquote>{c.detail}</blockquote>
					</div>
				{/each}
			{/if}

			{#if s.error}
				<pre class="log" style="color: var(--fail-fg); max-height: 200px;">{s.error}</pre>
			{/if}

			{#if activityCount(s) > 0}
				<details style="margin-top: 12px;">
					<summary class="muted" style="cursor: pointer;">
						Browser activity ({s.console_messages?.length ?? 0} console,
						{s.failed_requests?.length ?? 0} failed requests,
						{s.page_errors?.length ?? 0} JS errors)
					</summary>
					{#if s.console_messages?.length}
						<h3>Console</h3>
						<pre class="log">{s.console_messages.map((m) => `[${m.type}] ${m.text}`).join('\n')}</pre>
					{/if}
					{#if s.failed_requests?.length}
						<h3>Failed requests</h3>
						<pre class="log">{s.failed_requests
								.map((r) => `${r.status ?? r.failure} ${r.method} ${r.url}`)
								.join('\n')}</pre>
					{/if}
					{#if s.page_errors?.length}
						<h3>JS errors</h3>
						<pre class="log">{s.page_errors.join('\n')}</pre>
					{/if}
				</details>
			{/if}
		</article>
	{/each}
{:else if !error}
	<p class="muted">Loading…</p>
{/if}
