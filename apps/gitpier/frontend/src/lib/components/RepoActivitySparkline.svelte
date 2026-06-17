<script lang="ts">
	interface Props {
		series?: number[];
		label?: string;
	}

	const { series = [], label = 'Repository activity' }: Props = $props();

	const width = 112;
	const height = 30;
	const padX = 4;
	const padY = 3;

	function normalizeSeries(values: number[]): number[] {
		if (values.length > 0) return values;
		return new Array(12).fill(0);
	}

	function buildCoords(values: number[]): Array<{ x: number; y: number }> {
		if (values.length === 0) return [];
		const max = Math.max(1, ...values);
		const graphWidth = width - padX * 2;
		const graphHeight = height - padY * 2;
		const step = values.length > 1 ? graphWidth / (values.length - 1) : 0;

		return values.map((value, index) => ({
			x: padX + index * step,
			y: height - padY - (value / max) * graphHeight
		}));
	}

	function buildPath(coords: Array<{ x: number; y: number }>): string {
		if (coords.length === 0) return '';
		return coords.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(' ');
	}

	const points = $derived(normalizeSeries(series));
	const coords = $derived(buildCoords(points));
	const pathData = $derived(buildPath(coords));
</script>

<div class="w-28" aria-label={label}>
	<svg viewBox={`0 0 ${width} ${height}`} class="h-8 w-full overflow-visible" role="img" aria-label={label}>
		<line
			x1={padX}
			y1={height - padY}
			x2={width - padX}
			y2={height - padY}
			stroke="color-mix(in oklch, var(--border) 80%, transparent)"
			stroke-width="1"
		/>
		<path d={pathData} fill="none" stroke="var(--brand)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
	</svg>
</div>
