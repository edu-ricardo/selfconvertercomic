import { api } from './api';

/** Quantidade de tarefas abertas, mostrada no menu. Atualizada periodicamente e após ações. */
export const taskCount = $state({ open: 0 });

export async function refreshTaskCount() {
	try {
		taskCount.open = (await api.listTasks('open', 1)).open;
	} catch {
		// sem conexão: mantém o último valor
	}
}
