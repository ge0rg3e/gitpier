<script lang="ts">
	import type { LangStat } from '$lib/api/client';

	interface Props {
		languages: LangStat[];
		username: string;
	}

	const { languages, username }: Props = $props();

	const LANG_COLORS: Record<string, string> = {
		Go: '#00ADD8',
		JavaScript: '#f1e05a',
		TypeScript: '#3178c6',
		Python: '#3572A5',
		Ruby: '#701516',
		Rust: '#dea584',
		Java: '#b07219',
		Kotlin: '#A97BFF',
		Swift: '#F05138',
		'C#': '#178600',
		'C++': '#f34b7d',
		C: '#555555',
		PHP: '#4F5D95',
		Shell: '#89e051',
		HTML: '#e34c26',
		CSS: '#563d7c',
		Dart: '#00B4AB',
		Scala: '#c22d40',
		Haskell: '#5e5086',
		Elixir: '#6e4a7e',
		Vue: '#41b883',
		Svelte: '#ff3e00',
		Zig: '#ec915c',
		Nim: '#ffc200',
		Julia: '#a270ba',
		Lua: '#000080',
		R: '#198ce7',
		Nix: '#7e7eff',
		OCaml: '#3be133',
	};

	function langColor(lang: string): string {
		return LANG_COLORS[lang] ?? '#8b949e';
	}

	const total = $derived(languages.reduce((a, l) => a + l.count, 0));

	// Build donut segments
	const DONUT_R = 36;
	const DONUT_CIRC = 2 * Math.PI * DONUT_R;

	type Segment = { lang: LangStat; color: string; pct: number; offset: number; dash: number };

	const segments = $derived.by((): Segment[] => {
		if (total === 0) return [];
		let offset = 0;
		return languages.slice(0, 6).map((l) => {
			const pct = l.count / total;
			const dash = pct * DONUT_CIRC;
			const seg: Segment = { lang: l, color: langColor(l.name), pct, offset, dash };
			offset += dash;
			return seg;
		});
	});
</script>

<div class="rounded-xl border border-secondary bg-card p-5">
	<p class="text-sm font-semibold text-foreground mb-4">Top Languages</p>

	{#if languages.length === 0}
		<p class="text-sm text-muted-foreground text-center py-4">No language data yet.</p>
	{:else}
		<div class="flex items-center gap-5">
			<!-- Donut chart -->
			<div class="shrink-0">
				<svg width="88" height="88" viewBox="0 0 88 88">
					{#each segments as seg}
						<circle
							cx="44" cy="44" r={DONUT_R}
							fill="none"
							stroke={seg.color}
							stroke-width="14"
							stroke-dasharray="{seg.dash} {DONUT_CIRC - seg.dash}"
							stroke-dashoffset={-(seg.offset - DONUT_CIRC / 4)}
							transform="rotate(-90 44 44)"
						/>
					{/each}
					<!-- Center hole fill -->
					<circle cx="44" cy="44" r="22" fill="var(--color-card, #0d1117)" />
				</svg>
			</div>

			<!-- Legend -->
			<div class="flex-1 space-y-1.5 min-w-0">
				{#each languages.slice(0, 6) as lang}
					<div class="flex items-center gap-2 min-w-0">
						<span class="h-2.5 w-2.5 rounded-full shrink-0" style="background-color:{langColor(lang.name)}"></span>
						<span class="text-xs text-foreground truncate flex-1">{lang.name}</span>
						<span class="text-xs text-muted-foreground shrink-0">
							{total > 0 ? Math.round((lang.count / total) * 100) : 0}%
						</span>
					</div>
				{/each}
			</div>
		</div>
	{/if}
</div>
