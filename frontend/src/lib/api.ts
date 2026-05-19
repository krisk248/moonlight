// Tiny fetch wrapper for the Moonlight backend API.
// All endpoints are relative — the Svelte build is embedded into the Go binary
// and served from the same origin.

export interface StatusResponse {
	build_id: string;
	build_date: string;
	version: string;
	is_source_build: boolean;
	ai_enabled: boolean;
	ollama_up: boolean;
	ollama_model: string;
	ollama_model_present: boolean;
	revocation_active: boolean;
}

export interface SettingsResponse {
	ai_enabled: boolean;
	ollama_host: string;
	ollama_model: string;
	default_viewport_width: number;
	default_viewport_height: number;
	default_headless: boolean;
	default_timeout_ms: number;
	default_diff_tolerance: number;
}

export interface ScenarioSummary {
	name: string;
	url: string;
	steps: number;
	has_baseline: boolean;
	has_storage: boolean;
	needs_ai: boolean;
	tags: string[];
}

export interface Scenario {
	name: string;
	url: string;
	viewport?: { width: number; height: number };
	headless?: boolean;
	storage_state?: string;
	steps: Step[];
	tags?: string[];
}

export interface Step {
	action: string;
	name?: string;
	url?: string;
	selector?: string;
	role?: string;
	role_name?: string;
	label?: string;
	text?: string;
	value?: string;
	key?: string;
	ms?: number;
	y?: number;
	full_page?: boolean;
	ai_check?: { prompt: string; expect: string };
	dom_check?: {
		contains_text?: string[];
		selector_visible?: string;
		selector_hidden?: string;
		url_contains?: string;
		url_matches?: string;
	};
}

export interface RunSummary {
	id: string;
	scenario: string;
	passed: boolean;
	started_at: string;
	step_count: number;
	fail_count: number;
}

export interface StepResult {
	index: number;
	action: string;
	name: string;
	passed: boolean;
	error?: string;
	screenshot?: string;
	baseline?: string;
	diff_overlay?: string;
	diff_ratio?: number;
	ai_narration?: string;
	checks?: CheckResult[];
	console_messages?: { type: string; text: string; location: string }[];
	failed_requests?: { url: string; method: string; status?: number; failure?: string }[];
	page_errors?: string[];
}

export interface CheckResult {
	kind: string;
	passed: boolean;
	detail: string;
}

export interface RunResult {
	scenario: string;
	url: string;
	started_at: string;
	finished_at: string;
	passed: boolean;
	mode: string;
	ai_used: boolean;
	steps: StepResult[];
	run_dir: string;
	ai_summary?: string;
}

export interface Job {
	id: string;
	kind: string;
	label: string;
	status: 'running' | 'completed' | 'failed';
	log: string[];
	result?: RunResult;
	error?: string;
	started_at: string;
	finished_at?: string;
}

async function fetchJSON<T>(path: string, init?: RequestInit): Promise<T> {
	const r = await fetch(path, init);
	if (!r.ok) throw new Error(`${path} returned ${r.status}`);
	return r.json();
}

export const api = {
	status: () => fetchJSON<StatusResponse>('/api/status'),
	listScenarios: () => fetchJSON<ScenarioSummary[]>('/api/scenarios'),
	createScenario: (name: string, url: string) =>
		fetchJSON<Scenario>('/api/scenarios', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name, url })
		}),
	recordScenario: (name: string, url: string) =>
		fetchJSON<Job>('/api/scenarios/record', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ name, url })
		}),
	getScenario: (name: string) => fetchJSON<Scenario>(`/api/scenarios/${name}`),
	baseline: (name: string) => fetchJSON<Job>(`/api/scenarios/${name}/baseline`, { method: 'POST' }),
	run: (name: string) => fetchJSON<Job>(`/api/scenarios/${name}/run`, { method: 'POST' }),
	deleteScenario: (name: string) =>
		fetch(`/api/scenarios/${name}`, { method: 'DELETE' }).then((r) => r.ok),
	listRuns: () => fetchJSON<RunSummary[]>('/api/runs'),
	getRun: (id: string) => fetchJSON<RunResult>(`/api/runs/${id}`),
	getJob: (id: string) => fetchJSON<Job>(`/api/jobs/${id}`),
	getSettings: () => fetchJSON<SettingsResponse>('/api/settings'),
	updateSettings: (s: SettingsResponse) =>
		fetchJSON<SettingsResponse>('/api/settings', {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(s)
		})
};

// Convert an absolute server path returned by the API into a /files/* URL the
// browser can fetch from the Go static handler.
export function fileURL(absPath?: string): string | undefined {
	if (!absPath) return undefined;
	// Server paths look like /home/.../runs/<id>/... or /home/.../baselines/<name>/...
	const m = absPath.match(/\/(runs|baselines)\/(.*)$/);
	if (!m) return undefined;
	return `/files/${m[1]}/${m[2]}`;
}
