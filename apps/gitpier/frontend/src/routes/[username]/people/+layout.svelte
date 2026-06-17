<script lang="ts">
	let { children } = $props();
</script>

<div class="bg-background min-h-screen py-6 px-4">
	<div class="mx-auto max-w-screen-lg">
		{@render children()}
	</div>
</div>
