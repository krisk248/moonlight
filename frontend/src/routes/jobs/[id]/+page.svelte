<script lang="ts">
	import { page } from '$app/state';
	import { onMount, onDestroy } from 'svelte';
	import { goto } from '$app/navigation';
	import { api, type Job } from '$lib/api';

	let id = $derived(page.params.id);
	let job = $state<Job | null>(null);
	let error = $state('');
	let timer: ReturnType<typeof setInterval> | null = null;

	async function refresh() {
		try {
			job = await api.getJob(id);
			if (job.status === 'completed' && job.result?.run_dir) {
				// Extract run_id (last path segment) and redirect to the report.
				const parts = job.result.run_dir.replace(/\/+$/, '').split('/');
				const runId = parts[parts.length - 1];
				if (runId) {
					stop();
					goto(`/runs/${runId}`);
				}
			} else if (job.status === 'failed') {
				stop();
			}
		} catch (e) {
			error = (e as Error).message;
		}
	}

	function stop() {
		if (timer) {
			clearInterval(timer);
			timer = null;
		}
	}

	$effect(() => {
		if (id) {
			refresh();
			timer = setInterval(refresh, 1500);
			return () => stop();
		}
	});
</script>

{#if error}
	<div class="card" style="border-color: var(--fail);">
		<strong>Error:</strong>
		{error}
	</div>
{/if}

{#if job}
	<section class="card">
		<header>
			<div>
				<h2>
					{job.label}
					<span
						class="badge {job.status === 'completed'
							? 'pass'
							: job.status === 'failed'
								? 'fail'
								: 'pending'}"
					>
						{job.status}
					</span>
				</h2>
				<p class="muted" style="margin: 4px 0 0; font-size: 13px;">
					Job {job.id} · started {job.started_at}
					{#if job.finished_at}· finished {job.finished_at}{/if}
				</p>
			</div>
		</header>
		<pre class="log">{job.log.join('\n')}</pre>
		{#if job.error}
			<h3>Error</h3>
			<pre class="log" style="color: var(--fail-fg);">{job.error}</pre>
		{/if}
	</section>
{:else if !error}
	<p class="muted">Loading…</p>
{/if}
