<script lang="ts">
	import { tick } from 'svelte';

	interface Props {
		value: string;
		filePath?: string;
		containerClass?: string;
	}

	let { value = $bindable(''), filePath = '', containerClass = '' }: Props = $props();

	let textareaEl = $state<HTMLTextAreaElement | null>(null);

	const lineCount = $derived(value ? value.split('\n').length : 1);
	const lineNumbers = $derived(Array.from({ length: lineCount }, (_, i) => i + 1));

	function autoResize() {
		if (!textareaEl) return;
		textareaEl.style.height = '0px';
		textareaEl.style.height = `${textareaEl.scrollHeight}px`;
	}

	$effect(() => {
		void value;
		void tick().then(autoResize);
	});
</script>

<div class="editor-root {containerClass}">
	<div class="gutter" aria-hidden="true">
		{#each lineNumbers as n}
			<span class="line-num">{n}</span>
		{/each}
	</div>

	<div class="editor-area">
		<textarea bind:this={textareaEl} bind:value spellcheck="false" autocomplete="off" autocorrect="off" autocapitalize="off"></textarea>
	</div>
</div>

<style>
	.editor-root {
		display: flex;
		align-items: stretch;
		min-height: 500px;
		font-size: 13px;
		line-height: 20px;
		font-family: ui-monospace, SFMono-Regular, 'SF Mono', Menlo, Consolas, monospace;
	}

	.gutter {
		flex-shrink: 0;
		width: 35px;
		padding: 8px 5px 8px 0;
		text-align: right;
		background: inherit;
		position: sticky;
		left: 0;
		z-index: 1;
	}

	.line-num {
		display: block;
		height: 20px;
		line-height: 20px;
		font-size: 12px;
		color: lab(59.0737 -0.31975 -1.63626);
		user-select: none;
		white-space: nowrap;
	}

	.editor-area {
		flex: 1;
		min-width: 0;
	}

	textarea {
		display: block;
		width: 100%;
		min-height: 500px;
		margin: 0;
		padding: 8px 16px 8px 12px;
		font: inherit;
		line-height: inherit;
		white-space: pre;
		word-break: normal;
		overflow-wrap: normal;
		tab-size: 2;
		resize: none;
		border: none;
		outline: none;
		box-shadow: none;
		background: transparent;
		overflow: hidden;
	}
</style>
