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
			savedMessage = 'Saved. New defaults apply to all future runs.';
		} catch (e) {
			error = (e as Error).message;
		} finally {
			saving = false;
			setTimeout(() => (savedMessage = ''), 4000);
		}
	}

	// Common viewport presets for quick selection.
	const presets = [
		{ label: '1920 × 1080 (Full HD laptop/desktop)', w: 1920, h: 1080 },
		{ label: '1440 × 900 (MacBook-class laptop)', w: 1440, h: 900 },
		{ label: '1366 × 768 (older laptop)', w: 1366, h: 768 },
		{ label: '1280 × 720 (small / compact)', w: 1280, h: 720 },
		{ label: '2560 × 1440 (QHD)', w: 2560, h: 1440 }
	];

	function applyPreset(p: { w: number; h: number }) {
		if (!settings) return;
		settings.default_viewport_width = p.w;
		settings.default_viewport_height = p.h;
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
			<!-- ────────── Browser defaults ────────── -->
			<h3 class="section-h">Browser defaults</h3>
			<p class="muted small">
				Applied when a scenario YAML doesn't pin its own viewport / headless mode.
				Most testers should leave these at 1920×1080 headless — that's what laptops
				and desktops typically render at.
			</p>

			<div class="row col">
				<span class="title">Default viewport size</span>
				<div class="viewport-row">
					<div class="viewport-input">
						<label>Width</label>
						<input
							type="number"
							bind:value={settings.default_viewport_width}
							min="320"
							max="3840"
							step="10"
						/>
					</div>
					<span class="x">×</span>
					<div class="viewport-input">
						<label>Height</label>
						<input
							type="number"
							bind:value={settings.default_viewport_height}
							min="240"
							max="2160"
							step="10"
						/>
					</div>
					<select onchange={(e) => {
						const p = presets.find((p) => p.label === (e.target as HTMLSelectElement).value);
						if (p) applyPreset(p);
					}}>
						<option value="">Quick preset…</option>
						{#each presets as p}
							<option value={p.label}>{p.label}</option>
						{/each}
					</select>
				</div>
			</div>

			<label class="row">
				<div>
					<div class="title">Default headless mode</div>
					<div class="muted small">
						If on, Chromium runs invisibly during scenarios. Turn off to watch
						the browser drive itself (useful for debugging).
					</div>
				</div>
				<label class="switch">
					<input type="checkbox" bind:checked={settings.default_headless} />
					<span class="slider"></span>
				</label>
			</label>

			<!-- ────────── Runtime tuning ────────── -->
			<h3 class="section-h">Runtime tuning</h3>

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
					How long Playwright waits for an element before failing a click/fill (default
					15000 = 15 s). Individual steps can override with <code>timeout_ms: 30000</code>
					in the YAML.
				</span>
			</label>

			<label class="row col">
				<span class="title">Pixel diff tolerance (0–255)</span>
				<input
					type="number"
					bind:value={settings.default_diff_tolerance}
					min="0"
					max="255"
					step="1"
					placeholder="12"
				/>
				<span class="muted small">
					Maximum per-channel colour delta before a pixel is marked "changed".
					Lower = stricter (more failures from anti-aliasing noise). Higher =
					looser. 12 is a balanced default.
				</span>
			</label>

			<!-- ────────── AI ────────── -->
			<h3 class="section-h">AI verification</h3>

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
		gap: 14px;
		max-width: 760px;
	}
	.section-h {
		font-size: 13px;
		color: var(--muted);
		text-transform: uppercase;
		letter-spacing: 0.05em;
		margin: 20px 0 4px;
		padding-top: 12px;
		border-top: 1px solid var(--border);
	}
	.section-h:first-of-type {
		border-top: none;
		padding-top: 0;
		margin-top: 0;
	}
	.row {
		display: flex;
		gap: 16px;
		align-items: flex-start;
		justify-content: space-between;
		padding: 12px 0;
	}
	.row.col {
		flex-direction: column;
		align-items: stretch;
		gap: 6px;
	}
	.title {
		font-weight: 600;
		font-size: 14px;
	}
	.small {
		font-size: 12.5px;
		line-height: 1.4;
	}
	input[type='text'],
	input[type='number'] {
		background: var(--bg);
		color: var(--text);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 8px 12px;
		font-size: 14px;
		font-family: inherit;
		max-width: 520px;
		box-sizing: border-box;
	}
	select {
		background: var(--bg);
		color: var(--text);
		border: 1px solid var(--border);
		border-radius: 6px;
		padding: 8px 12px;
		font-size: 13px;
	}
	.viewport-row {
		display: flex;
		gap: 12px;
		align-items: flex-end;
		flex-wrap: wrap;
	}
	.viewport-input {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}
	.viewport-input label {
		font-size: 11px;
		color: var(--muted);
		text-transform: uppercase;
		letter-spacing: 0.04em;
	}
	.viewport-input input {
		width: 110px;
	}
	.x {
		font-size: 18px;
		color: var(--muted);
		padding-bottom: 8px;
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
	.actions {
		display: flex;
		gap: 12px;
		align-items: center;
		margin-top: 18px;
		padding-top: 14px;
		border-top: 1px solid var(--border);
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
