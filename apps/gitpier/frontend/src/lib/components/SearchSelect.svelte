<script lang="ts">
	import * as Popover from '$lib/components/ui/popover/index.js';
	import { ChevronDown, Check, Search } from '@lucide/svelte';
	import { cn } from '$lib/utils.js';

	// Height of each option row in px (py-1.5 + text-sm = 32px)
	const ITEM_HEIGHT = 32;
	// How many items to render in the virtual window
	const WINDOW = 40;
	// Extra items to render above/below the visible area as buffer
	const BUFFER = 8;

	interface Option {
		value: string;
		label?: string;
	}

	interface Props {
		value: string;
		options: Option[];
		placeholder?: string;
		class?: string;
		size?: 'sm' | 'default';
		onchange?: (value: string) => void;
	}

	let {
		value = $bindable(),
		options,
		placeholder = 'Select…',
		class: className,
		size = 'default',
		onchange
	}: Props = $props();

	let open = $state(false);
	let search = $state('');
	let inputEl = $state<HTMLInputElement | null>(null);
	let listEl = $state<HTMLDivElement | null>(null);
	let windowStart = $state(0);

	const selectedLabel = $derived(
		options.find((o) => o.value === value)?.label ??
		options.find((o) => o.value === value)?.value ??
		value ?? placeholder
	);

	// Full filtered list — never sliced, just filtered
	const filtered = $derived.by(() => {
		const q = search.trim().toLowerCase();
		if (!q) return options;
		return options.filter((o) => (o.label ?? o.value).toLowerCase().includes(q));
	});

	// Virtual window into filtered
	const windowEnd = $derived(Math.min(filtered.length, windowStart + WINDOW));
	const visibleItems = $derived(filtered.slice(windowStart, windowEnd));
	const topSpacer = $derived(windowStart * ITEM_HEIGHT);
	const bottomSpacer = $derived(Math.max(0, (filtered.length - windowEnd) * ITEM_HEIGHT));

	// Reset window to top when search query changes
	$effect(() => {
		// eslint-disable-next-line @typescript-eslint/no-unused-expressions
		search; // track
		windowStart = 0;
		listEl?.scrollTo({ top: 0 });
	});

	function onScroll(e: Event) {
		const el = e.currentTarget as HTMLDivElement;
		const scrollTop = el.scrollTop;
		const newStart = Math.max(0, Math.floor(scrollTop / ITEM_HEIGHT) - BUFFER);
		// Only update when we've moved enough to swap a row in/out
		if (Math.abs(newStart - windowStart) >= 1) {
			windowStart = newStart;
		}
	}

	function select(val: string) {
		value = val;
		open = false;
		search = '';
		onchange?.(val);
	}

	function onOpenChange(v: boolean) {
		open = v;
		if (v) {
			search = '';
			windowStart = 0;
			setTimeout(() => inputEl?.focus(), 0);
		}
	}
</script>

<Popover.Root {open} onOpenChange={onOpenChange}>
	<Popover.Trigger
		class={cn(
			'flex items-center justify-between gap-1.5 rounded-md border border-border bg-background text-sm font-semibold text-foreground focus:outline-none focus:ring-1 focus:ring-primary whitespace-nowrap',
			size === 'sm' ? 'h-7 pl-3 pr-2' : 'h-8 pl-3 pr-2',
			!value && 'text-muted-foreground font-normal',
			className
		)}
		aria-label="Select option"
	>
		{selectedLabel}
		<ChevronDown class="h-3 w-3 text-muted-foreground shrink-0" />
	</Popover.Trigger>
	<Popover.Content class="w-56 p-0 overflow-hidden" align="start" sideOffset={4}>
		<!-- Search input -->
		<div class="flex items-center gap-2 border-b border-border px-2 py-1.5">
			<Search class="h-3.5 w-3.5 text-muted-foreground shrink-0" />
			<input
				bind:this={inputEl}
				bind:value={search}
				type="text"
				placeholder="Search…"
				class="flex-1 bg-transparent text-sm text-foreground placeholder:text-muted-foreground focus:outline-none"
			/>
		</div>
		<!-- Virtual scrolling list -->
		<div bind:this={listEl} class="max-h-60 overflow-y-auto" onscroll={onScroll}>
			{#if filtered.length === 0}
				<p class="px-3 py-2 text-xs text-muted-foreground">No results.</p>
			{:else}
				<!-- Top spacer: represents unrendered items above -->
				<div style:height="{topSpacer}px" aria-hidden="true"></div>

				{#each visibleItems as opt (opt.value)}
					<button
						type="button"
						onclick={() => select(opt.value)}
						class={cn(
							'flex w-full items-center gap-2 px-2 py-1.5 text-sm text-foreground hover:bg-accent cursor-pointer',
							value === opt.value && 'bg-accent'
						)}
						style:height="{ITEM_HEIGHT}px"
					>
						<span class="flex-1 text-left truncate">{opt.label ?? opt.value}</span>
						{#if value === opt.value}
							<Check class="h-3.5 w-3.5 text-primary shrink-0" />
						{/if}
					</button>
				{/each}

				<!-- Bottom spacer: represents unrendered items below -->
				<div style:height="{bottomSpacer}px" aria-hidden="true"></div>
			{/if}
		</div>
	</Popover.Content>
</Popover.Root>

