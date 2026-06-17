<script lang="ts">
	import { onMount } from 'svelte';

	interface Props {
		code: string;
		filePath: string;
		containerClass?: string;
	}

	type FileContents = {
		name: string;
		contents: string;
	};

	type DiffsFileInstance = {
		render: (args: { file: FileContents; containerWrapper: HTMLElement }) => void;
		setThemeType: (theme: 'dark' | 'light') => void;
		cleanUp: () => void;
	};

	type DiffsFileCtor = new (args: {
		theme: { dark: string; light: string };
		themeType: 'dark' | 'light';
		overflow: 'scroll';
		disableFileHeader: boolean;
	}) => DiffsFileInstance;

	let { code, filePath, containerClass = '' }: Props = $props();

	let wrapper = $state<HTMLDivElement | null>(null);
	let fileInstance: DiffsFileInstance | null = null;
	let themeObserver: MutationObserver | null = null;

	const fileData = $derived(buildFile(code, filePath));

	function buildFile(contents: string, path: string): FileContents {
		return {
			name: path || 'file',
			contents: contents ?? ''
		};
	}

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
		if (!wrapper || !fileInstance) return;
		wrapper.innerHTML = '';
		fileInstance.render({
			file: fileData,
			containerWrapper: wrapper
		});
		applyBgToContainer();
	});

	onMount(() => {
		let disposed = false;

		void (async () => {
			const diffsModule = await import('@pierre/diffs');
			if (disposed) return;

			const DiffsFile = diffsModule.File as DiffsFileCtor;
			fileInstance = new DiffsFile({
				theme: { dark: 'github-dark', light: 'github-light' },
				themeType: getThemeType(),
				overflow: 'scroll',
				disableFileHeader: true
			});

			if (wrapper) {
				fileInstance.render({
					file: fileData,
					containerWrapper: wrapper
				});
				applyBgToContainer();
			}

			themeObserver = new MutationObserver(() => {
				const nextTheme = getThemeType();
				fileInstance?.setThemeType(nextTheme);
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
			fileInstance?.cleanUp();
			fileInstance = null;
		};
	});
</script>

{#if code?.length > 0}
	<div bind:this={wrapper} class={`border border-border overflow-x-auto bg-background rounded-md ${containerClass}`} style="--diffs-font-size: 12px; --diffs-line-height: 1.4;"></div>
{:else}
	<div class="border border-border px-4 py-3 text-xs text-muted-foreground bg-background rounded-md">No code content available.</div>
{/if}
