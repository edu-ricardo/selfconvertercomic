<script lang="ts">
	import PageHeader from '$lib/components/page-header.svelte';
	import EmptyState from '$lib/components/empty-state.svelte';
	import ListChecksIcon from '@lucide/svelte/icons/list-checks';
</script>

<PageHeader title="Tarefas" description="O que precisa da sua confirmação ou ajuste." />
<EmptyState
	icon={ListChecksIcon}
	title="Nada pendente"
	text="Erros de conversão e confirmações aparecerão aqui."
/>
