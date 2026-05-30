<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { webhooks, type Webhook, type WebhookDelivery } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { ArrowLeft, CheckCircle, XCircle, RefreshCw, Loader, ChevronDown, ChevronUp } from '@lucide/svelte';

	const username = $derived(page.params.username as string);
	const repoName = $derived(page.params.repo as string);
	const hookId = $derived(Number(page.params.id));

	const ALL_EVENTS = [
		{ value: 'push', label: 'Push', description: 'Any Git push to a repository' },
		{ value: 'issues', label: 'Issues', description: 'Issue opened, closed, or reopened' },
		{ value: 'issue_comment', label: 'Issue comments', description: 'Issue comment created' },
		{ value: 'pull_request', label: 'Pull requests', description: 'PR opened, closed, or merged' },
		{ value: 'pull_request_review', label: 'PR reviews', description: 'Pull request review submitted' },
		{ value: 'release', label: 'Releases', description: 'Release published or updated' },
		{ value: 'create', label: 'Branch/tag created', description: 'Branch or tag created' },
		{ value: 'delete', label: 'Branch/tag deleted', description: 'Branch or tag deleted' }
	];

	let hook = $state<Webhook | null>(null);
	let deliveries = $state<WebhookDelivery[]>([]);
	let loading = $state(true);
	let error = $state('');

	// Edit state
	let payloadURL = $state('');
	let contentType = $state('application/json');
	let secret = $state('');
	let insecureSSL = $state(false);
	let active = $state(true);
	let eventMode = $state<'push' | 'all' | 'select'>('push');
	let selectedEvents = $state<Set<string>>(new Set(['push']));
	let saving = $state(false);
	let saveError = $state('');
	let saveSuccess = $state(false);

	// Delivery expansion
	let expandedDelivery = $state<number | null>(null);
	let redelivering = $state<number | null>(null);

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		try {
			const [hookData, deliveriesData] = await Promise.all([webhooks.get(username, repoName, hookId), webhooks.deliveries.list(username, repoName, hookId).catch(() => ({ deliveries: [] }))]);
			hook = hookData.webhook;
			deliveries = deliveriesData.deliveries ?? [];
			initForm(hook);
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	function initForm(h: Webhook) {
		payloadURL = h.payload_url;
		contentType = h.content_type;
		insecureSSL = h.insecure_ssl;
		active = h.active;

		if (h.events.includes('*')) {
			eventMode = 'all';
		} else if (h.events.length === 1 && h.events[0] === 'push') {
			eventMode = 'push';
		} else {
			eventMode = 'select';
			selectedEvents = new Set(h.events);
		}
	}

	function toggleEvent(value: string) {
		const s = new Set(selectedEvents);
		if (s.has(value)) s.delete(value);
		else s.add(value);
		selectedEvents = s;
	}

	function resolvedEvents(): string[] {
		if (eventMode === 'push') return ['push'];
		if (eventMode === 'all') return ['*'];
		return [...selectedEvents];
	}

	async function handleSave(e: Event) {
		e.preventDefault();
		saving = true;
		saveError = '';
		saveSuccess = false;
		try {
			const updated = await webhooks.update(username, repoName, hookId, {
				payload_url: payloadURL.trim(),
				content_type: contentType,
				secret: secret || undefined,
				insecure_ssl: insecureSSL,
				active,
				events: resolvedEvents()
			});
			hook = updated.webhook;
			saveSuccess = true;
			secret = '';
			setTimeout(() => (saveSuccess = false), 3000);
		} catch (e: any) {
			saveError = e.message;
		} finally {
			saving = false;
		}
	}

	async function handleRedeliver(deliveryId: number) {
		redelivering = deliveryId;
		try {
			await webhooks.deliveries.redeliver(username, repoName, hookId, deliveryId);
			// refresh deliveries
			const data = await webhooks.deliveries.list(username, repoName, hookId);
			deliveries = data.deliveries ?? [];
		} catch (e: any) {
			alert(e.message);
		} finally {
			redelivering = null;
		}
	}

	function formatDate(s: string) {
		return new Date(s).toLocaleString();
	}
</script>

<div class="container mx-auto max-w-3xl px-4 py-8">
	<a href={`/${username}/${repoName}/settings/webhooks`} class="text-muted-foreground mb-6 flex items-center gap-1 text-sm hover:underline">
		<ArrowLeft class="h-4 w-4" />
		Webhooks
	</a>

	{#if loading}
		<div class="text-muted-foreground py-12 text-center text-sm">Loading…</div>
	{:else if error}
		<div class="text-destructive py-12 text-center text-sm">{error}</div>
	{:else if hook}
		<h1 class="mb-1 font-mono text-lg font-semibold">{hook.payload_url}</h1>
		<div class="text-muted-foreground mb-8 text-xs">
			Created {formatDate(hook.created_at)} · Last updated {formatDate(hook.updated_at)}
		</div>

		<!-- Edit form -->
		<form onsubmit={handleSave} class="mb-10 space-y-6">
			<h2 class="text-lg font-semibold">Settings</h2>

			<div class="space-y-2">
				<Label for="payload-url">Payload URL <span class="text-destructive">*</span></Label>
				<Input id="payload-url" type="url" bind:value={payloadURL} required />
			</div>

			<div class="space-y-2">
				<Label for="content-type">Content type</Label>
				<select
					id="content-type"
					bind:value={contentType}
					class="border-input bg-background focus-visible:ring-ring w-full rounded-md border px-3 py-2 text-sm focus-visible:ring-2 focus-visible:outline-none"
				>
					<option value="application/json">application/json</option>
					<option value="application/x-www-form-urlencoded">application/x-www-form-urlencoded</option>
				</select>
			</div>

			<div class="space-y-2">
				<Label for="secret">Secret</Label>
				<Input id="secret" type="password" placeholder={hook.has_secret ? '••••••••  (leave blank to keep existing)' : 'Optional'} bind:value={secret} />
			</div>

			<div class="space-y-3">
				<Label>SSL verification</Label>
				<div class="space-y-2">
					<label class="flex cursor-pointer items-center gap-2">
						<input type="radio" name="ssl" value={false} bind:group={insecureSSL} class="accent-primary" />
						<span class="text-sm font-medium">Enable SSL verification</span>
					</label>
					<label class="flex cursor-pointer items-center gap-2">
						<input type="radio" name="ssl" value={true} bind:group={insecureSSL} class="accent-primary" />
						<span class="text-sm font-medium text-yellow-600">Disable (not recommended)</span>
					</label>
				</div>
			</div>

			<div class="space-y-3">
				<Label>Which events would you like to trigger this webhook?</Label>
				<div class="space-y-2">
					<label class="flex cursor-pointer items-center gap-2">
						<input type="radio" name="events" value="push" bind:group={eventMode} class="accent-primary" />
						<span class="text-sm">Just the <strong>push</strong> event.</span>
					</label>
					<label class="flex cursor-pointer items-center gap-2">
						<input type="radio" name="events" value="all" bind:group={eventMode} class="accent-primary" />
						<span class="text-sm">Send me <strong>everything</strong>.</span>
					</label>
					<label class="flex cursor-pointer items-center gap-2">
						<input type="radio" name="events" value="select" bind:group={eventMode} class="accent-primary" />
						<span class="text-sm">Let me <strong>select individual events</strong>.</span>
					</label>
				</div>

				{#if eventMode === 'select'}
					<div class="bg-muted/40 mt-3 grid grid-cols-1 gap-2 rounded-lg border p-4 sm:grid-cols-2">
						{#each ALL_EVENTS as evt}
							<label class="flex cursor-pointer items-start gap-2">
								<Checkbox checked={selectedEvents.has(evt.value)} onCheckedChange={() => toggleEvent(evt.value)} class="mt-0.5" />
								<div>
									<p class="text-sm font-medium">{evt.label}</p>
									<p class="text-muted-foreground text-xs">{evt.description}</p>
								</div>
							</label>
						{/each}
					</div>
				{/if}
			</div>

			<div class="flex items-start gap-3">
				<Checkbox id="active" bind:checked={active} class="mt-0.5" />
				<div>
					<label for="active" class="cursor-pointer text-sm font-medium">Active</label>
					<p class="text-muted-foreground text-xs">We will deliver event details when this hook is triggered.</p>
				</div>
			</div>

			{#if saveError}
				<p class="text-destructive text-sm">{saveError}</p>
			{/if}
			{#if saveSuccess}
				<p class="text-sm text-green-600">Webhook updated successfully.</p>
			{/if}

			<Button type="submit" disabled={saving}>
				{saving ? 'Saving…' : 'Update webhook'}
			</Button>
		</form>

		<!-- Recent deliveries -->
		<div>
			<h2 class="mb-4 text-lg font-semibold">Recent deliveries</h2>

			{#if deliveries.length === 0}
				<p class="text-muted-foreground text-sm">No deliveries yet.</p>
			{:else}
				<div class="divide-y rounded-lg border">
					{#each deliveries as delivery (delivery.id)}
						<div class="overflow-hidden">
							<!-- Delivery summary row -->
							<button
								class="flex w-full items-center gap-3 p-4 text-left hover:bg-muted/40 transition-colors"
								onclick={() => (expandedDelivery = expandedDelivery === delivery.id ? null : delivery.id)}
							>
								{#if delivery.success}
									<CheckCircle class="h-4 w-4 shrink-0 text-green-500" />
								{:else}
									<XCircle class="text-destructive h-4 w-4 shrink-0" />
								{/if}

								<code class="text-muted-foreground text-xs">{delivery.guid}</code>

								<Badge variant="secondary" class="text-xs">{delivery.event}</Badge>

								<span class="text-muted-foreground ml-auto shrink-0 text-xs">
									{delivery.response_code > 0 ? delivery.response_code : '—'}
									· {delivery.duration_ms}ms · {formatDate(delivery.created_at)}
								</span>

								{#if expandedDelivery === delivery.id}
									<ChevronUp class="h-4 w-4 shrink-0" />
								{:else}
									<ChevronDown class="h-4 w-4 shrink-0" />
								{/if}
							</button>

							<!-- Expanded detail -->
							{#if expandedDelivery === delivery.id}
								<div class="border-t bg-muted/20 px-4 pb-4 pt-3 space-y-4">
									<div class="flex justify-end">
										<Button variant="outline" size="sm" onclick={() => handleRedeliver(delivery.id)} disabled={redelivering === delivery.id}>
											{#if redelivering === delivery.id}
												<Loader class="mr-2 h-3 w-3 animate-spin" />
											{:else}
												<RefreshCw class="mr-2 h-3 w-3" />
											{/if}
											Redeliver
										</Button>
									</div>

									<div>
										<p class="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Payload sent</p>
										<pre class="bg-muted rounded p-3 text-xs overflow-x-auto whitespace-pre-wrap break-all">{(() => {
												try {
													return JSON.stringify(JSON.parse(delivery.payload), null, 2);
												} catch {
													return delivery.payload;
												}
											})()}</pre>
									</div>

									{#if delivery.response_body}
										<div>
											<p class="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">
												Response (HTTP {delivery.response_code})
											</p>
											<pre class="bg-muted rounded p-3 text-xs overflow-x-auto whitespace-pre-wrap break-all">{delivery.response_body}</pre>
										</div>
									{/if}
								</div>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
