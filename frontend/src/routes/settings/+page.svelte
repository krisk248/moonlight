<script lang="ts">
	import { onMount } from 'svelte';
	import { api, type SettingsResponse } from '$lib/api';

	let settings = $state<SettingsResponse | null>(null);
	let error = $state('');
	let saving = $state(false);
	let savedMessage = $state('');

	async function load() {
		try {
			settings = await api.getSettings();
		} catch (e) {
			error = (e as Error).message;
		}
	}

	async function save() {
		if (!settings) return;
		saving = true;
		savedMessage = '';
		try {
			settings = await api.updateSettings(settings);
			savedMessage = 'Saved. Reload other tabs to see changes.';
		} catch (e) {
			error = (e as Error).message;
		} finally {
			saving = false;
			setTimeout(() => (savedMessage = ''), 4000);
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
		<h2>Settings</h2>
	</header>

	{#if settings}
		<div class="form">
			<label class="row">
				<div>
					<div class="title">Enable AI assistance</div>
					<div class="muted small">
						Off by default. When enabled, scenarios may carry <code>ai_check</code> blocks that
						send screenshots to a local Ollama server for yes/no verification. Recording, playback,
						pixel diff, and DOM-based assertions all work without this.
					</div>
				</div>
				<label class="switch">
					<input type="checkbox" bind:checked={settings.ai_enabled} />
					<span class="slider"></span>
				</label>
			</label>

			{#if settings.ai_enabled}
				<div class="ai-block">
					<label class="row col">
						<span class="title">Ollama host</span>
						<input type="text" bind:value={settings.ollama_host} placeholder="http://127.0.0.1:11434" />
						<span class="muted small">Where the Ollama server is reachable.</span>
					</label>
					<label class="row col">
						<span class="title">Vision model</span>
						<input
							type="text"
							bind:value={settings.ollama_model}
							placeholder="ahmadwaqar/smolvlm2-2.2b-instruct"
						/>
						<span class="muted small">
							The model name as it appears in <code>ollama list</code>. Pull it first:
							<code>ollama pull {settings.ollama_model || 'model-name'}</code>
						</span>
					</label>
				</div>
			{/if}

			<label class="row col">
				<span class="title">Default action timeout (ms)</span>
				<input
					type="number"
					bind:value={settings.default_timeout_ms}
					min="1000"
					step="1000"
					placeholder="15000"
				/>
				<span class="muted small">
					How long Playwright waits for an element before failing a click/fill (default 15000 = 15 s).
					Individual steps can override with <code>timeout_ms: 30000</code> in the YAML.
				</span>
			</label>

			<div class="actions">
				<button class="btn primary" disabled={saving} onclick={save}>
					{saving ? 'Saving…' : 'Save settings'}
				</button>
				{#if savedMessage}<span class="muted small" style="color: var(--pass-fg);">{savedMessage}</span>{/if}
			</div>
		</div>
	{:else if !error}
		<p class="muted">Loading…</p>
	{/if}
</section>

<style>
	.form {
		display: flex;
		flex-direction: column;
		gap: 18px;
		max-width: 720px;
	}
	.row {
		display: flex;
		gap: 16px;
		align-items: flex-start;
		justify-content: space-between;
		padding: 14px 0;
		border-bottom: 1px solid var(--border);
	}
	.row.col {
		flex-direction: column;
		align-items: stretch;
		gap: 6px;
	}
	.row:last-child {
		border-bottom: 0;
	}
	.title {
		font-weight: 600;
		font-size: 14px;
	}
	.small {
		font-size: 12.5px;
	}
	.ai-block {
		padding: 14px 18px;
		background: var(--bg);
		border: 1px solid var(--border);
		border-radius: 8px;
		display: flex;
		flex-direction: column;
		gap: 14px;
	}
	.ai-block input {
		background: var(--surface);
		color: var(--text);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 8px 12px;
		font-size: 14px;
		font-family: inherit;
		width: 100%;
		box-sizing: border-box;
	}
	.actions {
		display: flex;
		gap: 12px;
		align-items: center;
		margin-top: 8px;
	}

	.switch {
		position: relative;
		display: inline-block;
		width: 44px;
		height: 24px;
		flex-shrink: 0;
	}
	.switch input {
		opacity: 0;
		width: 0;
		height: 0;
	}
	.slider {
		position: absolute;
		cursor: pointer;
		inset: 0;
		background-color: #555;
		transition: 0.2s;
		border-radius: 24px;
	}
	.slider::before {
		position: absolute;
		content: '';
		height: 18px;
		width: 18px;
		left: 3px;
		bottom: 3px;
		background-color: #fff;
		transition: 0.2s;
		border-radius: 50%;
	}
	.switch input:checked + .slider {
		background-color: var(--accent);
	}
	.switch input:checked + .slider::before {
		transform: translateX(20px);
	}
</style>
