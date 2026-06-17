<script lang="ts">
	import { onMount } from 'svelte';

	interface Props {
		patch: string;
		filePath: string;
		diffStyle?: 'unified' | 'split';
	}

	type FileDiffInstance = {
		options: Record<string, unknown>;
		setOptions: (options: Record<string, unknown>) => void;
		render: (args: { fileDiff: unknown; containerWrapper: HTMLElement; forceRender?: boolean }) => void;
		setThemeType: (theme: 'dark' | 'light') => void;
		cleanUp: () => void;
	};

	type FileDiffCtor = new (args: {
		theme: { dark: string; light: string };
		themeType: 'dark' | 'light';
		diffStyle: 'unified' | 'split';
		overflow: 'scroll';
		disableFileHeader: boolean;
	}) => FileDiffInstance;

	type ParsePatchFiles = (rawPatch: string, oldFileName?: string, strict?: boolean) => Array<{ files?: unknown[] }>;

	const { patch, filePath, diffStyle = 'unified' }: Props = $props();

	let wrapper = $state<HTMLDivElement | null>(null);
	let diffInstance: FileDiffInstance | null = null;
	let themeObserver: MutationObserver | null = null;
	let parsePatchFiles: ParsePatchFiles | null = null;

	const normalizedPatch = $derived(buildRenderablePatch(patch, filePath));
	const hasPatch = $derived(normalizedPatch.length > 0);
	let fileDiff = $state<unknown | null>(null);
	let parseError = $state('');

	function buildRenderablePatch(rawPatch: string, path: string) {
		if (!rawPatch.trim()) return '';
		const hasHeaders = /^(--- |\+\+\+ )/m.test(rawPatch);
		if (hasHeaders) return rawPatch;
		const safePath = path || 'file';
		return `--- a/${safePath}\n+++ b/${safePath}\n${rawPatch}`;
	}

	function refreshParsedDiff() {
		if (!parsePatchFiles || !normalizedPatch) {
			fileDiff = null;
			parseError = '';
			return;
		}

		try {
			const parsed = parsePatchFiles(normalizedPatch, undefined, true);
			fileDiff = parsed[0]?.files?.[0] ?? null;
			parseError = '';
		} catch (error) {
			fileDiff = null;
			parseError = error instanceof Error ? error.message : 'Failed to parse diff';
		}
	}

	$effect(() => {
		if (!wrapper || !diffInstance || !fileDiff) return;
		diffInstance.setOptions({
			...diffInstance.options,
			diffStyle
		});
		diffInstance.render({
			fileDiff,
			containerWrapper: wrapper,
			forceRender: true
		});
		applyBgToContainer();
	});

	function getThemeType(): 'dark' | 'light' {
		if (typeof document === 'undefined') return 'dark';
		return document.documentElement.classList.contains('dark') ? 'dark' : 'light';
	}

	function getResolvedBg(): string {
		const raw = getComputedStyle(document.documentElement).getPropertyValue('--background').trim();
		return raw || (getThemeType() === 'dark' ? '#1a1a2e' : '#ffffff');
	}

	function applyBgToContainer() {
		if (!wrapper) return;
		const container = wrapper.querySelector('diffs-container') as HTMLElement | null;
		if (!container) return;
		const bg = getResolvedBg();
		container.style.setProperty('--diffs-bg', bg);
		container.style.setProperty('--diffs-dark-bg', bg);
		container.style.setProperty('--diffs-light-bg', bg);
		container.style.setProperty('background-color', 'transparent');
	}

	$effect(() => {
		normalizedPatch;
		refreshParsedDiff();
	});

	onMount(() => {
		let disposed = false;

		void (async () => {
			const diffsModule = await import('@pierre/diffs');
			if (disposed) return;

			parsePatchFiles = diffsModule.parsePatchFiles as ParsePatchFiles;
			const FileDiff = diffsModule.FileDiff as unknown as FileDiffCtor;

			refreshParsedDiff();

			diffInstance = new FileDiff({
				theme: { dark: 'github-dark', light: 'github-light' },
				themeType: getThemeType(),
				diffStyle: diffStyle,
				overflow: 'scroll',
				disableFileHeader: true
			});

			if (wrapper && fileDiff) {
				diffInstance.render({
					fileDiff,
					containerWrapper: wrapper
				});
				applyBgToContainer();
			}

			themeObserver = new MutationObserver(() => {
				const nextTheme = getThemeType();
				diffInstance?.setThemeType(nextTheme);
				applyBgToContainer();
			});
			themeObserver.observe(document.documentElement, {
				attributes: true,
				attributeFilter: ['class', 'data-theme']
			});
		})();

		return () => {
			disposed = true;
			themeObserver?.disconnect();
			themeObserver = null;
			diffInstance?.cleanUp();
			diffInstance = null;
		};
	});
</script>

{#if parseError}
	<div class="border-t border-border px-4 py-3 text-xs text-muted-foreground bg-background">Unable to render diff: {parseError}</div>
{:else if hasPatch}
	<div bind:this={wrapper} class="border-t border-border overflow-x-auto bg-background" style="--diffs-font-size: 12px; --diffs-line-height: 1.4;"></div>
{:else}
	<div class="border-t border-border px-4 py-3 text-xs text-muted-foreground bg-background">No diff content available.</div>
{/if}
