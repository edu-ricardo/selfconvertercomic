<script lang="ts">
	import PageHeader from '$lib/components/page-header.svelte';
	import EmptyState from '$lib/components/empty-state.svelte';
	import ListOrderedIcon from '@lucide/svelte/icons/list-ordered';
</script>

<PageHeader title="Fila" description="Conversões em andamento e aguardando." />
<EmptyState
	icon={ListOrderedIcon}
	title="Fila vazia"
	text="A fila de conversões chega no marco M3."
/>
